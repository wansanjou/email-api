package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	gomail "gopkg.in/gomail.v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var db *gorm.DB

type User struct {
	ID       uint      `gorm:"primaryKey"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Products []Product `json:"products"`
}

type Product struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `json:"name"`
	Expiry    time.Time `json:"expiry"`
	UserID    uint      `json:"user_id"`
	CreatedAt time.Time
}

func main() {
	var err error
	db, err = gorm.Open(sqlite.Open("products.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect database")
	}

	// migrate ทั้ง User และ Product
	db.AutoMigrate(&User{}, &Product{})

	r := gin.Default()

	// User APIs
	r.POST("/users", createUser)
	r.GET("/users", listUsers)

	// Product APIs ต่อกับ user
	r.POST("/users/:id/products", addProductToUser)
	r.GET("/users/:id/products", listUserProducts)

	// ตั้ง cron ให้รันทุกวัน 8 โมงเช้า
	c := cron.New()
	_, err = c.AddFunc("@every 1m", checkExpiryJob)
	if err != nil {
		log.Fatal("cron error:", err)
	}
	c.Start()

	log.Println("Server started on :8081")
	r.Run(":8081")
}

// สมัคร user
func createUser(c *gin.Context) {
	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := User{Name: input.Name, Email: input.Email}
	db.Create(&user)
	c.JSON(http.StatusOK, user)
}

// list users
func listUsers(c *gin.Context) {
	var users []User
	db.Find(&users)
	c.JSON(http.StatusOK, users)
}

// เพิ่ม product ให้ user
func addProductToUser(c *gin.Context) {
	var input struct {
		Name   string    `json:"name"`
		Expiry time.Time `json:"expiry"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user User
	if err := db.First(&user, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	product := Product{Name: input.Name, Expiry: input.Expiry, UserID: user.ID}
	db.Create(&product)

	c.JSON(http.StatusOK, product)
}

// list products ของ user
func listUserProducts(c *gin.Context) {
	var products []Product
	if err := db.Where("user_id = ?", c.Param("id")).Find(&products).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found or no products"})
		return
	}
	c.JSON(http.StatusOK, products)
}

func addProduct(c *gin.Context) {
	var input struct {
		Name   string    `json:"name"`
		Expiry time.Time `json:"expiry"` // ส่งแบบ 2025-10-08T15:04:05Z
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	product := Product{Name: input.Name, Expiry: input.Expiry}
	db.Create(&product)

	c.JSON(http.StatusOK, product)
}

func listProducts(c *gin.Context) {
	var products []Product
	db.Find(&products)
	c.JSON(http.StatusOK, products)
}

func sendEmail(productName string, daysLeft int) {
	m := gomail.NewMessage()
	m.SetHeader("From", "your-email@gmail.com")
	m.SetHeader("To", "receiver@gmail.com")
	m.SetHeader("Subject", "สินค้าใกล้หมดอายุ")
	m.SetBody("text/plain", fmt.Sprintf("สินค้า %s จะหมดอายุใน %d วัน", productName, daysLeft))

	d := gomail.NewDialer("smtp.gmail.com", 587, "your-email@gmail.com", "your-app-password")

	if err := d.DialAndSend(m); err != nil {
		log.Println("ส่งอีเมลผิดพลาด:", err)
	} else {
		log.Println("ส่งอีเมลแล้ว:", productName)
	}
}

func checkExpiryJob() {
	log.Println("Running expiry check job...")

	var users []User
	db.Preload("Products").Find(&users)

	now := time.Now()
	for _, u := range users {
		var expiring []string
		for _, p := range u.Products {
			daysLeft := int(p.Expiry.Sub(now).Hours() / 24)
			if daysLeft <= 3 {
				expiring = append(expiring, fmt.Sprintf("%s (เหลือ %d วัน)", p.Name, daysLeft))
			}
		}

		// ถ้ามีสินค้าที่ใกล้หมดอายุ → ส่งอีเมล
		if len(expiring) > 0 {
			log.Printf("เตรียมส่งอีเมลไปที่ %s : %v\n", u.Email, expiring)
			sendEmailToUser(u.Email, expiring)
		}
	}
}

func sendEmailToUser(email string, products []string) {
	m := gomail.NewMessage()
	m.SetHeader("From", "your-email@gmail.com") // ต้องเป็น Gmail จริง
	m.SetHeader("To", email)
	m.SetHeader("Subject", "แจ้งเตือนสินค้าของคุณใกล้หมดอายุ")

	body := "รายการสินค้าที่ใกล้หมดอายุ:\n"
	for _, p := range products {
		body += "- " + p + "\n"
	}
	m.SetBody("text/plain", body)

	// ใช้ App Password ของ Gmail
	d := gomail.NewDialer("smtp.gmail.com", 587, "watcharapol2c@gmail.com", "ombc hhai loun juam")

	if err := d.DialAndSend(m); err != nil {
		log.Println("❌ ส่งอีเมลผิดพลาด:", err)
	} else {
		log.Println("✅ ส่งอีเมลแล้ว:", email)
	}
}
