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
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"math/rand"
	"time"
	"log"
	"github.com/go-redis/redis/v9"
	// "golang.design/x/clipboard"
)

func NewClient(ctx context.Context) *redis.Client {
	client := redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // no password set
			DB:       0,  // use default DB
	})

	pong, err := client.Ping(ctx).Result()
	if err != nil {
			panic(err)
	}

	fmt.Println(pong)
	return client
}

// main
func main() {
	// create gin server
  server := gin.Default()
	server.SetTrustedProxies([]string{"127.0.0.1"})

	ctx := context.Background()

	// create mongodb server
	mgdb, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		panic(err)
	}

	rds := NewClient(ctx)

	// ping mongodb to check connection
	if err := mgdb.Ping(context.TODO(), readpref.Primary()); err != nil {
		panic(err)
	}
	// Declare Context type object for managing multiple API requests
	// ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)

	server.Use(mgMiddleware(mgdb))
	server.Use(rdsMiddleware(rds))

	server.LoadHTMLGlob("template/*")
	server.Static("/static", "./static")
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
	ctx := context.TODO()
	url, exist := c.GetPostForm("inputURL")
	if(!exist) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "URL Shortner",
			// "Content": "none",
			// "error": "fail",
		})
	} else {
		mgdb := c.Request.Context().Value("mgdb").(*mongo.Client)
		rds := c.Request.Context().Value("rds").(*redis.Client)
		newKey := generateURL(url)
		// write to mongodb
		record := URL{
					Keys: newKey,
					Origin: url,
		}
		collection := mgdb.Database("shortner").Collection("urlMapping")
		collection.InsertOne(ctx, record)

		//write to redis & set expire time to 1 week
		err := rds.Set(ctx, newKey, url, 1*time.Minute).Err()
		if err != nil {
			log.Println("Redis.Set failed", err)
		}

		// dbClient := c.Request.Context().Value("client").(*mongo.Client)
		host := "127.0.0.1:8080/"
		c.HTML(http.StatusOK, "result.html", gin.H{
			"title": "URL Shortner",
			"short": host + newKey,
			"origin": url,
		})
	}
}

func redirectHandler(c *gin.Context) {
	rds := c.Request.Context().Value("rds").(*redis.Client)
	key := c.Param("shortUrl")
	notInReids := false
	// check redis
	ctx := context.TODO()
	origin, err := rds.Get(ctx, key).Result()
	if err == redis.Nil {
			// fmt.Println("origin url does not exist")
			notInReids = true
	} else if err != nil {
			fmt.Println("client.Get failed", err)
	} else {
			// fmt.Println("key2", val2)
			c.Redirect(http.StatusMovedPermanently, origin)
	}

	if notInReids == true {
		mgdb := c.Request.Context().Value("mgdb").(*mongo.Client)
		collection := mgdb.Database("shortner").Collection("urlMapping")
		var result bson.M
		lookFor := bson.D{{"keys", key}}
		err := collection.FindOne(context.TODO(), lookFor).Decode(&result)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				// no match
				c.HTML(http.StatusOK, "index.html", gin.H{
					"title": "URL Shortner",
				})
			} 	
		} else {
				// match
				originURL := result["origin"].(string)
				c.Redirect(http.StatusMovedPermanently, originURL)

				//write to redis & set expire time to 1 week
				err := rds.Set(ctx, key, originURL, 1*time.Minute).Err()
				if err != nil {
					log.Println("Redis.Set failed", err)
				}
		}
	}
}

func generateURL(url string) string {
	head, tail := randomString()
	sha := sha256.New()
	sha.Write([]byte(head + url + tail))
	encoded := base62.EncodeToString(sha.Sum(nil))
	return string(encoded[:8])
}

// Middleware will add the db client to the context
func mgMiddleware(cl *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "mgdb", cl))
		c.Next()
	}
}
func rdsMiddleware(cl *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "rds", cl))
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
	Keys			string							`bson:"keys,omitempty"`
	Origin		string							`bson:"origin,omitempty"`
}

