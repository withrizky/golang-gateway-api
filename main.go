package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/joho/godotenv"
)

//go:embed public
var uiFiles embed.FS

func main() {
	// 1. Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Peringatan: File .env tidak ditemukan, menggunakan setting default")
	}

	// 2. Debug: Cek apakah file index.html benar-benar ada di embed
	// Ini untuk memastikan build Anda sukses
	testFile, err := uiFiles.ReadFile("public/index.html")
	if err != nil {
		log.Printf("CRITICAL ERROR: File public/index.html tidak ditemukan di embed! %v", err)
	} else {
		log.Printf("SUCCESS: Embed berhasil membaca index.html (%d bytes)", len(testFile))
	}

	app := fiber.New(fiber.Config{
		AppName:      "API Gateway",
		ServerHeader: "By Rizky",
		BodyLimit:    2 * 1024 * 1024, // max 2 mb file
	})

	// --- MIDDLEWARE KEAMANAN ---
	app.Use(helmet.New())
	app.Use(cors.New())
	app.Use(logger.New())

	app.Use(limiter.New(limiter.Config{
		Max:        50,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	}))

	// --- ROUTE 1: DASHBOARD (PORTAL UTAMA) ---
	dist, err := fs.Sub(uiFiles, "public")
	if err != nil {
		log.Fatal("Gagal membuat sub-filesystem:", err)
	}

	// Serve Static Files
	app.Use("/", filesystem.New(filesystem.Config{
		Root:   http.FS(dist),
		Index:  "index.html",
		Browse: false,
	}))

	// --- ROUTE 2: API GATEWAY (THE GUARDIAN) ---
	apiGateway := app.Group("/v1", authMiddleware)

	// Forward ke WhatsApp Service
	apiGateway.All("/wa/*", func(c *fiber.Ctx) error {
		target := os.Getenv("WA_SERVICE_URL") + "/" + c.Params("*")
		log.Printf("[PROXY] Forwarding to WhatsApp: %s", target)
		return proxy.Do(c, target)
	})

	// Forward ke Mail Service
	apiGateway.All("/mail/*", func(c *fiber.Ctx) error {
		target := os.Getenv("MAIL_SERVICE_URL") + "/" + c.Params("*")
		log.Printf("[PROXY] Forwarding to Mail: %s", target)
		return proxy.Do(c, target)
	})

	// Health Check
	app.Get("/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "Guarding", "uptime": "Active"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	log.Printf("RizGate Ultimate berjalan di port %s", port)
	log.Fatal(app.Listen(":" + port))
}

func authMiddleware(c *fiber.Ctx) error {
	key := c.Get("X-RIZ-KEY")
	secret := os.Getenv("RIZ_SECRET_KEY")

	if key == "" || key != secret {
		log.Printf("[SECURITY ALERT] Unauthorized access attempt from IP: %s", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Akses Ditolak. API Key tidak valid.",
		})
	}
	return c.Next()
}
