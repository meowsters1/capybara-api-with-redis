package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/looskie/capybara-api/utils"
	v1 "github.com/looskie/capybara-api/v1"
	"github.com/redis/go-redis/v9"
)

func ping(r *redis.Client) {
	for range time.Tick(time.Second * 10) {
		log.Println("Pinging")

		if err := r.Ping(context.Background()).Err(); err != nil {
			log.Fatal(err.Error())
			break
		}
	}
}

func main() {
	godotenv.Load()

	containerId, err := os.Hostname()
	if err != nil {
		containerId = "UNKNOWN"
	}

	capyImages, _ := os.ReadDir("capys")
	utils.NUMBER_OF_IMAGES = len(capyImages)

	if err := utils.LoadCapyAlts("utils/alt.json"); err != nil {
		log.Printf("could not load alt text, using default response: %s", err)
	}

	var tlsObj *tls.Config = nil
	if os.Getenv("REDIS_SECURE_SKIP") == "true" {
		tlsObj = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:      os.Getenv("REDIS"),
		Username:  os.Getenv("REDIS_USERNAME"),
		Password:  os.Getenv("REDIS_PASSWORD"),
		DB:        0,
		TLSConfig: tlsObj,
	})

	go ping(rdb)

	println("Attempting connection at ", rdb.Options().Addr, rdb.Options().Username, rdb.Options().Password, rdb.Options().DB)

	app := fiber.New(fiber.Config{
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"10.50.0.0/24"},
	})

	app.Use(recover.New(recover.Config{
		Next:             nil,
		EnableStackTrace: true,
	}))

	app.Use(logger.New(logger.Config{
		Format: "${time} |   ${cyan}${status} ${reset}|   ${latency} | ${ip} on ${cyan}${ua} ${reset}| ${cyan}${method} ${reset}${path} \n",
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET",
	}))

	app.Use(limiter.New(limiter.Config{
		Max:        500,
		Expiration: 30 * time.Second,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(utils.Response{
				Success: false,
				Message: "You are being rate limited",
			})
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.GetReqHeaders()["X-Forwarded-For"]
		},
	}))

	app.Use(func(c *fiber.Ctx) error {
		error := rdb.Incr(context.Background(), "visits").Err()

		if error != nil {
			println(error.Error())
		}

		return c.Next()
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(utils.Response{
			Success: true,
			Message: "ok you pull up (Powered by hop.io) " + containerId,
		})
	})

	v1Group := app.Group("/v1")
	v1Group.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(utils.Response{
			Success: true,
			Message: "welcome to v1 of capybara heaven (Powered by hop.io) " + containerId,
		})
	})

	v1Group.Get("/capybaras", v1.GetCapybaras)
	v1Group.Get("/capybara", v1.GetCapybara)
	v1Group.Get("/capybara/:index", v1.GetCapybaraByIndex)
	v1Group.Get("/capyoftheday", v1.GetCapybaraOfTheDay)
	v1Group.Get("/capyhour", v1.GetCapyHour)
	v1Group.Get("/capyofthehour", v1.GetCapyHour) // Alias

	// Capybara facts

	v1Group.Get("/fact", v1.GetCapyFact)
	v1Group.Get("/facts", v1.GetCapyFacts)

	var port = os.Getenv("PORT")

	if len(port) == 0 {
		port = "3000"
	}

	log.Fatal(app.Listen(":" + port))
}
