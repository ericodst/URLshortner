package main

import (
	"fmt"
  "net/http"
  "github.com/gin-gonic/gin"
	"crypto/sha256"
	"github.com/jxskiss/base62"
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/x/bsonx"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"math/rand"
	"time"
	"log"
	"strconv"
)

// main
func main() {
	// create gin server
  server := gin.Default()
	server.SetTrustedProxies([]string{"127.0.0.1"})
	// create mongodb server
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}
	// ping mongodb to check connection
	if err := client.Ping(context.TODO(), readpref.Primary()); err != nil {
		panic(err)
	}
	// Declare Context type object for managing multiple API requests
	// ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)

	server.Use(dbMiddleware(client))

	server.LoadHTMLGlob("template/*")
	server.GET("/", indexHandler)
	server.POST("/new", getShortURL)
	server.GET("/:shortUrl", redirectHandler)

	server.Run(":8080")
}

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "URL Shortner",
	})
}

func getShortURL(c *gin.Context) {
	url, exist := c.GetPostForm("inputURL")
	if(!exist) {
		c.HTML(http.StatusOK, "test.html", gin.H{
			"title": "Error",
			"Content": "none",
			"error": "fail",
		})
	} else {
		dbClient := c.Request.Context().Value("client").(*mongo.Client)
		c.HTML(http.StatusOK, "test.html", gin.H{
			"title": "URL Shortner",
			"Content": checkExist(c, url, dbClient),
			"success": url,
		})
	}
}

func redirectHandler(c *gin.Context) {
	short := c.Param("shortUrl")
	client := c.Request.Context().Value("client").(*mongo.Client)
	collection := client.Database("shortner").Collection("urlMapping")
	var result bson.M
	lookFor := bson.D{{"short", short}}
	err := collection.FindOne(context.TODO(), lookFor).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// no match
			c.HTML(http.StatusOK, "index.html", gin.H{
				"title": "URL Shortner",
				"error": "This short url is not exist",
			})
		} 	
	} else {
			// match
			originURL := result["origin"].(string)
			c.Redirect(http.StatusMovedPermanently, originURL)
	}
}

func checkExist(c *gin.Context, url string, client *mongo.Client) string {
	ctx := context.TODO()
	// ctx := context.Background()
	shortURL := ""
	setTime, exist := c.GetPostForm("time-setting")
	days := 7
	hours := 0
	if(setTime == "on" && exist) { 
		day, _ := c.GetPostForm("lifeTime-days")
		hour, _ := c.GetPostForm("lifeTime-hours")
		days, _ = strconv.Atoi(day)
		hours, _ = strconv.Atoi(hour)
	}

	collection := client.Database("shortner").Collection("urlMapping")

	if setTime == "on" {
		shortURL = generateURL(url)
		item := URL{
			Origin: url,
			Short:	shortURL,
			Custom: "true",
			CreateAt: time.Now().UTC(),
			ExpireAt:	int32(3600 * (days * 24 + hours)),
		}
		expire := mongo.IndexModel{
			Keys: bson.M{"createdAt": bsonx.Int32(int32(time.Now().Unix()))},
			Options: options.Index().SetExpireAfterSeconds(int32(3600 * (days * 24 + hours))),
			// Options: options.Index().SetExpireAfterSeconds(60),
		}
		_, err := collection.Indexes().CreateOne(ctx, expire)
		if err != nil {
				log.Fatal(err)
		} 
		collection.InsertOne(ctx, item)
	} else {
		var result bson.M
		lookFor := bson.D{{"origin", url}, {"custom", "false"}}
		// lookForCustom := bson.D{{"custom", "false"}}
		// opts := options.FindOne().SetProjection(lookForCustom)

		err := collection.FindOne(context.TODO(), lookFor).Decode(&result)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				// no match
				shortURL = generateURL(url)
				item := URL{
					Origin: url,
					Short:	shortURL,
					Custom: "false",
					CreateAt: time.Now().UTC(),
					ExpireAt:	604800,
				}
				expire := mongo.IndexModel{
					Keys: bson.M{"createdAt": bsonx.Int32(int32(time.Now().Unix()))},
					Options: options.Index().SetExpireAfterSeconds(604800), // 7 days
					// Options: options.Index().SetExpireAfterSeconds(60),
				}
				_, err := collection.Indexes().CreateOne(ctx, expire)
				if err != nil {
						log.Fatal(err)
				} 
				collection.InsertOne(ctx, item)
			} else {
				fmt.Println("error")
				log.Fatal(err)
				// match
				// fmt.Println(shortURL)
				// shortURL = result["short"].(string)
				// fmt.Println(shortURL)
				// cmd := bson.D{{"collmod", collection}, {"key", {"createAt",result["short"].(string)}}}
			}
		} else {
			// match
			fmt.Println(result)
			shortURL = result["short"].(string)
			// fmt.Println("error")
			// log.Fatal(err)
		}
	}

	// var result bson.M
	// lookFor := bson.D{{"origin", url}}
	// err := collection.FindOne(context.TODO(), lookFor).Decode(&result)
	// if err != nil {
	// 	if err == mongo.ErrNoDocuments {
	// 		shortURL = generateURL(url)
	// 		// no match
	// 		if(setTime == "on") {
	// 			item := URL{
	// 				Origin: url,
	// 				Short:	shortURL,
	// 				Custom: "true",
	// 				CreateAt: time.Now().UTC(),
	// 				ExpireAt:	60,
	// 			}
	// 			expire := mongo.IndexModel{
	// 				Keys: bson.M{"createdAt": bsonx.Int32(int32(time.Now().Unix()))},
	// 				Options: options.Index().SetExpireAfterSeconds(3600 * (days * 24 + hours)),
	// 				// Options: options.Index().SetExpireAfterSeconds(60),
	// 			}
	// 			cr, err := collection.Indexes().CreateOne(ctx, expire)
	// 			if err != nil {
	// 					log.Fatal(err)
	// 			} 
	// 			collection.InsertOne(ctx, item)
	// 		} else {
	// 			item := URL{
	// 				Origin: url,
	// 				Short:	shortURL,
	// 				Custom: "false",
	// 				CreateAt: time.Now().UTC(),
	// 				ExpireAt:	60,
	// 			}
	// 			expire := mongo.IndexModel{
	// 				Keys: bson.M{"createdAt": bsonx.Int32(int32(time.Now().Unix()))},
	// 				// Keys: bson.M{"createdAt": time.Now()},
	// 				Options: options.Index().SetExpireAfterSeconds(604800),  // 7 days
	// 				// Options: options.Index().SetExpireAfterSeconds(60),
	// 			}
	// 			_, err := collection.Indexes().CreateOne(ctx, expire)
	// 			if err != nil {
	// 				log.Fatal(err)
	// 			}
	// 			// doc := bson.D{{"origin", url}, {"short", shortURL}, {"custom", "false"}}
	// 			collection.InsertOne(ctx, item)
	// 		}
	// 		// shortURL = generateURL(url)
	// 		// doc := bson.D{{"origin", url}, {"short", shortURL}, }
	// 		// collection.InsertOne(context.TODO(), doc)
	// 	} 	
	// } else {
	// 		// match
	// 		if(setTime == "on") {
	// 			item := URL{
	// 				Origin: url,
	// 				Short:	shortURL,
	// 				Custom: "true",
	// 				CreateAt: time.Now().UTC(),
	// 				ExpireAt:	60,
	// 			}
	// 			expire := mongo.IndexModel{
	// 				Keys: bson.M{"createdAt": bsonx.Int32(int32(time.Now().Unix()))},
	// 				Options: options.Index().SetExpireAfterSeconds(3600 * (days * 24 + hours)),
	// 				// Options: options.Index().SetExpireAfterSeconds(60),
	// 			}
	// 			cr, err := collection.Indexes().CreateOne(ctx, expire)
	// 			if err != nil {
	// 					log.Fatal(err)
	// 			} 
	// 			collection.InsertOne(ctx, item)
	// 		} else {
	// 			shortURL = result["short"].(string)
	// 			// TO-DO: reset life time
	// 		}
	// 		// shortURL = result["short"].(string)
	// }

	return shortURL
}

func generateURL(url string) string {
	head, tail := randomString()
	sha := sha256.New()
	sha.Write([]byte(head + url + tail))
	encoded := base62.EncodeToString(sha.Sum(nil))
	return string(encoded[:8])
}

// dbMiddleware will add the db client to the context
func dbMiddleware(cl *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "client", cl))
		c.Next()
	}
}

func randomString() (string, string) {
	var charset = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rand.Seed(time.Now().UnixNano())
	head := make([]byte, 4)
	tail := make([]byte, 4)
	for i := 0 ; i < 4 ; i++ {
		head[i] = charset[rand.Intn(len(charset))]
		tail[i] = charset[rand.Intn(len(charset))]
	}
	return string(head), string(tail)
} 

type URL struct {
	ID				primitive.ObjectID	`bson:"_id,omitempty"`
	Origin		string							`bson:"origin,omitempty"`
	Short			string							`bson:"short,omitempty"`
	Custom 		string							`bson:"custom,omitempty"`
	CreateAt  time.Time								`bson:"createdAt,omitempty"`
	ExpireAt	int32								`json:"expireAt" bson:"expireAt"`
}
