package main

import (
  "net/http"
  "github.com/gin-gonic/gin"
	"crypto/sha256"
	"github.com/jxskiss/base62"
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
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

	server.Use(dbMiddleware(client))

	server.LoadHTMLGlob("template/*")
	server.GET("/", indexHandler)
	server.POST("/new", getShortURL)

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
			"Content": checkExist(url, dbClient),
			"success": url,
		})
	}
}

func generateURL(url string) string {
	sha := sha256.New()
	sha.Write([]byte(url))
	encoded := base62.EncodeToString(sha.Sum(nil))
	return string(encoded[:8])
}

func checkExist(url string, client *mongo.Client) string {
	shortURL := ""
	collection := client.Database("shortner").Collection("urlMapping")
	var result bson.M
	lookFor := bson.D{{"origin", url}}
	err := collection.FindOne(context.TODO(), lookFor).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// no match
			shortURL = generateURL(url)
			doc := bson.D{{"origin", url}, {"short", shortURL}}
			collection.InsertOne(context.TODO(), doc)
		} 	
	} else {
			// match
			shortURL = result["short"].(string)
	}

	return shortURL
}

// dbMiddleware will add the db connection to the context
func dbMiddleware(cl *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "client", cl))
		c.Next()
	}
}

