package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/gocolly/colly"
	"github.com/joho/godotenv"
	"github.com/mindsgn-studio/amazon-scraper/category"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client
var ctx context.Context
var totalPages int = 1
var total uint64 = 0

type Price struct {
	ItemID   string    `bson:"ItemID"`
	Date     time.Time `bson:"date"`
	Currency string    `bson:"currency"`
	Price    float64   `bson:"price"`
}

func connectDatabase() error {
	ctx = context.Background()
	err := godotenv.Load()
	if err != nil {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	mongoURI := os.Getenv("MONGODB_URI")
	mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return fmt.Errorf("error connecting to MongoDB: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = mongoClient.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("error pinging MongoDB: %w", err)
	}

	fmt.Println("Connected to MongoDB successfully")
	return nil
}

func saveItemPrice(price float64, title string, link string) {
	db := mongoClient.Database("snapprice")
	itemCollection := db.Collection("items")
	pricesCollection := db.Collection("prices")

	twelveHoursAgo := time.Now().Add(-12 * time.Hour)

	filter := map[string]interface{}{
		"title": title,
		"link":  link,
	}

	var result map[string]interface{}

	err := itemCollection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return
	}

	if id, ok := result["_id"].(primitive.ObjectID); ok {
		itemID := id.Hex()
		filter := map[string]interface{}{
			"itemID": itemID,
			"date":   map[string]interface{}{"$gt": twelveHoursAgo},
		}

		var result map[string]interface{}
		err := pricesCollection.FindOne(ctx, filter).Decode(&result)
		if err != nil {
			newPrice := &Price{
				ItemID:   itemID,
				Date:     time.Now(),
				Currency: "zar",
				Price:    price,
			}

			_, err := pricesCollection.InsertOne(ctx, newPrice)
			if err != nil {
				fmt.Println("failed to save Price")
				return
			}
		}
	}

	return
}

func extractPrice(text string) (float64, error) {
	re := regexp.MustCompile(`\d+\.\d+`)
	match := re.FindString(text)
	price, err := strconv.ParseFloat(match, 64)

	if err != nil {
		return 0, fmt.Errorf("Error parsing price")
	}

	return price, nil
}

func saveItemData(title string, images []string, link string, id string) {
	db := mongoClient.Database("snapprice")
	collection := db.Collection("items")

	var filter = map[string]interface{}{
		"sources.id": id,
	}

	var update = map[string]interface{}{
		"$set": map[string]interface{}{
			"title":   title,
			"images":  images,
			"link":    link,
			"updated": time.Now(),
			"sources": map[string]interface{}{
				"id":     id,
				"source": "amazon",
			},
		},
	}

	upsert := true

	_, err := collection.UpdateOne(ctx, filter, update, &options.UpdateOptions{Upsert: &upsert})
	if err != nil {
		fmt.Println(err)
		return
	}
}

func getPage(brand string, page int) {
	collyClient := colly.NewCollector()
	var link = fmt.Sprintf("https://www.amazon.co.za/s?k=%s&page=%d", url.QueryEscape(brand), page)

	collyClient.OnHTML("div.s-result-list.s-search-results.sg-row", func(h *colly.HTMLElement) {
		h.ForEach("div.a-section.a-spacing-base", func(_ int, cardElement *colly.HTMLElement) {
			var name string
			var itemLink string
			var itemID string = "3232"
			var images []string
			var price float64

			h.ForEach("div.sg-col-4-of-24.sg-col-4-of-12.s-result-item.s-asin.sg-col-4-of-16.sg-col.s-widget-spacing-small.sg-col-4-of-20", func(_ int, cardParent *colly.HTMLElement) {
				itemID = cardParent.Attr("data-asin")
			})

			name = cardElement.ChildText("span.a-size-base-plus.a-color-base.a-text-normal")

			text := cardElement.ChildText("span.a-offscreen")
			price, err := extractPrice(text)
			if err != nil {
				return
			}

			cardElement.ForEach("a.a-link-normal.s-no-outline", func(_ int, h *colly.HTMLElement) {
				itemLink = "https://www.amazon.co.za/" + h.Attr("href")
			})

			cardElement.ForEach("img.s-image", func(_ int, h *colly.HTMLElement) {
				images = append(images, h.Attr("src"))
			})

			saveItemData(name, images, itemLink, itemID)
			saveItemPrice(price, name, itemLink)
		})

		h.ForEach("span.s-pagination-item.s-pagination-disabled", func(_ int, h *colly.HTMLElement) {
			if h.Text != "Previous" {
				number, err := strconv.Atoi(h.Text)
				if err != nil {
					return
				}

				if page == 1 {
					totalPages = number
				}

			}
		})
	})

	collyClient.Visit(link)
	collyClient.Wait()

	if page >= totalPages {
		fmt.Println(time.Now(), "Total Items:", total)
		totalPages = 1
		total = 0
		getBrand()
	} else {
		page++
		getPage(brand, page)
	}
}

func getBrand() {
	brand := category.GetRandomCategory()
	fmt.Println(time.Now(), brand)
	getPage(brand, 1)
}

func main() {
	err := connectDatabase()
	if err != nil {
		panic(err)
	}

	getBrand()
}
