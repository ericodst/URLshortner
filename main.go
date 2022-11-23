package main

import (
	"fmt"
	"os"
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
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"net/url"
	// "github.com/joho/godotenv"
)

func NewClient(ctx context.Context) *redis.Client {
	client := redis.NewClient(&redis.Options{
			Addr:     "redis:6379",
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
	mgdb, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://mongodb:27017"))
	if err != nil {
		panic(err)
	}

	rds := NewClient(ctx)

	if err := mgdb.Ping(ctx, readpref.Primary()); err != nil {
		panic(err)
	}

	// line bot
	bot := lineConnect()
	

	// server routes
	server.Use(mgMiddleware(mgdb))
	server.Use(rdsMiddleware(rds))
	server.Use(lineMiddleware(bot))

	server.LoadHTMLGlob("/template/*")
	server.Static("/asset", "../asset")
	server.GET("/", indexHandler)
	server.POST("/new", getShortURL)
	server.GET("/:shortUrl", redirectHandler)
	// linebot 
	server.POST("/", botHandler)

	server.Run(":8080")
}

func lineConnect() *linebot.Client {
	// err := godotenv.Load(".env")
  // if err != nil {
	// 	log.Println("godotenv error")
  //   log.Fatal(err)
  // }
	channelSecret := os.Getenv("CHANNELSECRET")
	channelToekn := os.Getenv("CHANNELTOKEN")
	lineClient := &http.Client{}
	bot, err := linebot.New(channelSecret, channelToekn, linebot.WithHTTPClient(lineClient))
	if err != nil {
		log.Println(err)
	}

	return bot
}

// linebot handler
func botHandler(c *gin.Context){
	ctx := context.TODO()
	bot := c.Request.Context().Value("linebot").(*linebot.Client)
	events, err := bot.ParseRequest(c.Request)
	if err != nil {
		log.Println(err)
	}

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			// only handle text message
			case *linebot.TextMessage:
				// check whether the input url is valid
				_, err := url.ParseRequestURI(message.Text)
				if err != nil {
					_, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("It's not a valid url")).Do()
					if err != nil {
						log.Println(err)
					}
				} else {
					log.Println(event.Message)
					mgdb := c.Request.Context().Value("mgdb").(*mongo.Client)
					rds := c.Request.Context().Value("rds").(*redis.Client)
					newKey := generateURL(message.Text)
					// write to mongodb
					record := URL{
						Keys: newKey,
						Origin: message.Text,
					}
					collection := mgdb.Database("shortner").Collection("urlMapping")
					collection.InsertOne(ctx, record)

					//write to redis & set expire time to 1 week
					err := rds.Set(ctx, newKey, message.Text, 1*time.Minute).Err()
					if err != nil {
						log.Println("Redis.Set failed", err)
					}
					host := "127.0.0.1:8080/"
					_, er := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(host+newKey)).Do()
					if er != nil {
						log.Println(er)
					}
				}
			}
		}
	}
}

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "URL Shortner",
	})
}

func getShortURL(c *gin.Context) {
	ctx := context.TODO()
	inputUrl, exist := c.GetPostForm("inputURL")
	if(!exist) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"title": "URL Shortner",
		})
	} else {
		mgdb := c.Request.Context().Value("mgdb").(*mongo.Client)
		rds := c.Request.Context().Value("rds").(*redis.Client)
		newKey := generateURL(inputUrl)
		// write to mongodb
		record := URL{
			Keys: newKey,
			Origin: inputUrl,
		}
		collection := mgdb.Database("shortner").Collection("urlMapping")
		collection.InsertOne(ctx, record)

		//write to redis & set expire time to 1 week
		err := rds.Set(ctx, newKey, inputUrl, 1*time.Minute).Err()
		if err != nil {
			log.Println("Redis.Set failed", err)
		}

		host := "127.0.0.1:3000/"
		c.HTML(http.StatusOK, "result.html", gin.H{
			"title": "URL Shortner",
			"short": host + newKey,
			"origin": inputUrl,
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
			notInReids = true
	} else if err != nil {
			fmt.Println("client.Get failed", err)
	} else {
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
func lineMiddleware(cl *linebot.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "linebot", cl))
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

