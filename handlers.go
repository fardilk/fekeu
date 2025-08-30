package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"be03/models"
	"be03/pkg/ocr"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

func setupRoutes(r *gin.Engine) {
	r.POST("/register", registerHandler)
	r.POST("/login", loginHandler)
	r.POST("/refresh", refreshHandler)
	r.POST("/revoke_refresh", revokeRefreshHandler)
	authGroup := r.Group("")
	authGroup.Use(jwtAuthMiddleware())
	authGroup.GET("/me", meHandler)
	authGroup.POST("/catatan", createCatatanHandler)
	authGroup.GET("/catatan", listCatatanHandler)
	authGroup.GET("/catatan/revenue", revenueSummaryHandler)
	authGroup.POST("/profile", createProfileHandler)
	authGroup.GET("/profile", getProfileHandler)
	authGroup.POST("/uploads", uploadFileHandler)
	authGroup.GET("/uploads", listUploadsHandler)
	authGroup.GET("/uploads/:id", getUploadHandler)
}

func jwtAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
			c.Abort()
			return
		}
		tokenString := authHeader[7:]
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrInvalidKeyType
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			c.Abort()
			return
		}
		username, _ := claims["username"].(string)
		role, _ := claims["role"].(string)
		c.Set("username", username)
		if role != "" {
			c.Set("role", role)
		}
		c.Next()
	}
}

func meHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	if usernameVal == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "context missing username"})
		return
	}
	username := usernameVal.(string)
	c.JSON(http.StatusOK, gin.H{"username": username})
}

// getUserFromContext fetches the currently authenticated user using the username set by jwtAuthMiddleware
func getUserFromContext(c *gin.Context) (*models.User, bool) {
	unameVal, _ := c.Get("username")
	if unameVal == nil {
		return nil, false
	}
	uname := unameVal.(string)
	var user models.User
	if err := db.Where("username = ?", uname).First(&user).Error; err != nil {
		return nil, false
	}
	return &user, true
}

// createCatatanHandler creates a CatatanKeuangan for the authenticated user
func createCatatanHandler(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var req struct {
		FileName string `json:"file_name" binding:"required"`
		Amount   int64  `json:"amount" binding:"required"`
		Date     string `json:"date"` // optional ISO8601
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// prevent duplicate file for the same user
	var existing models.CatatanKeuangan
	if err := db.Where("user_id = ? AND file_name = ?", user.ID, req.FileName).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "file already recorded"})
		return
	}

	ct := models.CatatanKeuangan{UserID: user.ID, FileName: req.FileName, Amount: req.Amount}
	if req.Date != "" {
		if t, err := time.Parse(time.RFC3339, req.Date); err == nil {
			ct.Date = t
		}
	} else {
		ct.Date = time.Now()
	}
	if err := db.Create(&ct).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": ct.ID})
}

// listCatatanHandler lists recent catatan for the authenticated user (admin sees all)
func listCatatanHandler(c *gin.Context) {
	role, _ := c.Get("role")
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var items []models.CatatanKeuangan
	q := db.Model(&models.CatatanKeuangan{})
	if role != "administrator" {
		q = q.Where("user_id = ?", user.ID)
	}
	if err := q.Order("id desc").Limit(200).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	c.JSON(http.StatusOK, items)
}

// revenueSummaryHandler returns a simple sum of Amount grouped by month
func revenueSummaryHandler(c *gin.Context) {
	role, _ := c.Get("role")
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	type Result struct {
		Month string
		Total int64
	}
	var results []Result
	// Basic SQL to group by month (works on sqlite and postgres)
	q := db.Model(&models.CatatanKeuangan{})
	if role != "administrator" {
		q = q.Where("user_id = ?", user.ID)
	}
	// Use to_char for Postgres to group by YYYY-MM
	rows, err := q.Select("to_char(date, 'YYYY-MM') as month, sum(amount) as total").Group("month").Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r Result
		rows.Scan(&r.Month, &r.Total)
		results = append(results, r)
	}
	c.JSON(http.StatusOK, results)
}

func registerHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := Register(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user registered successfully"})
}

func createProfileHandler(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var req struct {
		Name       string `json:"name" binding:"required"`
		Address    string `json:"address"`
		Email      string `json:"email"`
		Phone      string `json:"phone"`
		Occupation string `json:"occupation"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	profile := models.Profile{UserID: user.ID, Name: req.Name, Address: req.Address, Email: req.Email, Phone: req.Phone, Occupation: req.Occupation}
	if err := db.Create(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": profile.ID})
}

func getProfileHandler(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var p models.Profile
	if err := db.Where("user_id = ?", user.ID).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	// Generate JWT token. Resolve role name from RoleID (we only store role_id now).
	roleName := ""
	if user.RoleID != nil {
		var r models.Role
		if err := db.First(&r, *user.RoleID).Error; err == nil {
			roleName = r.Name
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"role":     roleName,
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	// create refresh token
	refreshToken, err := createAndStoreRefreshToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create refresh token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "login successful", "token": tokenString, "refresh_token": refreshToken})
}

// createAndStoreRefreshToken generates a random refresh token, stores its hash with expiry and returns the raw token string
func createAndStoreRefreshToken(userID uint) (string, error) {
	// generate random 32-byte token (hex)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	// hash for storage
	h := sha256.Sum256([]byte(token))
	th := hex.EncodeToString(h[:])
	rt := models.RefreshToken{UserID: userID, TokenHash: th, ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	if err := db.Create(&rt).Error; err != nil {
		return "", err
	}
	return token, nil
}

// helper to find refresh token record by raw token string
func findRefreshTokenByRaw(token string) (*models.RefreshToken, error) {
	h := sha256.Sum256([]byte(token))
	th := hex.EncodeToString(h[:])
	var rt models.RefreshToken
	if err := db.Where("token_hash = ?", th).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

// refreshHandler exchanges a refresh token for a new access token and rotates the refresh token
func refreshHandler(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rt, err := findRefreshTokenByRaw(req.RefreshToken)
	if err != nil || rt.Revoked || time.Now().After(rt.ExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	// load user
	var user models.User
	if err := db.First(&user, rt.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	// create access token
	roleName := ""
	if user.RoleID != nil {
		var r models.Role
		if err := db.First(&r, *user.RoleID).Error; err == nil {
			roleName = r.Name
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"role":     roleName,
		"exp":      time.Now().Add(15 * time.Minute).Unix(),
	})
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	// rotate refresh token: revoke existing and create new one
	db.Model(&models.RefreshToken{}).Where("id = ?", rt.ID).Update("revoked", true)
	newRT, err := createAndStoreRefreshToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate refresh token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tokenString, "refresh_token": newRT})
}

// revokeRefreshHandler revokes a given refresh token (useful on logout)
func revokeRefreshHandler(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rt, err := findRefreshTokenByRaw(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "refresh token not found"})
		return
	}
	rt.Revoked = true
	if err := db.Save(rt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "refresh token revoked"})
}

// uploadFileHandler handles multipart image upload for the current user's profile.
func uploadFileHandler(c *gin.Context) {
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	// ensure profile exists
	var profile models.Profile
	if err := db.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile missing"})
		return
	}
	folder := c.PostForm("folder")
	if folder == "" {
		folder = "default"
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file missing"})
		return
	}
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 5MB)"})
		return
	}
	// simple content type sniff via header
	ct := file.Header.Get("Content-Type")
	baseDir := uploadBaseDir()
	relPath := folder + "/" + file.Filename
	fullPath := baseDir + "/" + relPath
	if err := os.MkdirAll(baseDir+"/"+folder, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkdir failed"})
		return
	}
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}
	var keuID *uint
	if v := c.PostForm("keuangan_id"); v != "" {
		// try to parse uint
		var parsed uint64
		parsed, _ = strconv.ParseUint(v, 10, 64)
		if parsed != 0 {
			pv := uint(parsed)
			keuID = &pv
		}
	}
	// Build store path (public exposure path). Assume files served from 'public/' prefix.
	storePath := "public/" + relPath
	// Optional amount to auto-create catatan keuangan
	var catatanID *uint
	if amtStr := c.PostForm("amount"); amtStr != "" {
		if amtVal, err := strconv.ParseInt(amtStr, 10, 64); err == nil && amtVal > 0 {
			// check duplicate
			var existing models.CatatanKeuangan
			if err := db.Where("user_id = ? AND file_name = ?", user.ID, file.Filename).First(&existing).Error; err == nil {
				// already exists, link to existing
				keuID = &existing.ID
				cid := existing.ID
				catatanID = &cid
			} else {
				ck := models.CatatanKeuangan{UserID: user.ID, FileName: file.Filename, Amount: amtVal, Date: time.Now()}
				if err := db.Create(&ck).Error; err == nil {
					cid := ck.ID
					catatanID = &cid
					keuID = &cid
				}
			}
		}
	}
	// If an upload record for this profile+filename already exists, return it
	var existingUp models.Upload
	if err := db.Where("profile_id = ? AND file_name = ?", profile.ID, file.Filename).First(&existingUp).Error; err == nil {
		// If existing upload has no keuangan link, try OCR and link if possible (but avoid creating duplicate Catatan)
		if existingUp.KeuanganID == nil {
			if amt, conf, found, err := ocr.ExtractAmountFromImage(fullPath); err == nil && amt > 0 && conf > 0.15 {
				// Normalize only if original OCR matched a decimal-like cents pattern
				if found != "" {
					lf := strings.TrimSpace(found)
					if centsRE.MatchString(lf) {
						if amt%100 == 0 {
							amt = amt / 100
						}
					}
				}
				var existingCat models.CatatanKeuangan
				if err := db.Where("user_id = ? AND file_name = ?", profile.UserID, existingUp.FileName).First(&existingCat).Error; err == nil {
					existingUp.KeuanganID = &existingCat.ID
					db.Save(&existingUp)
				} else {
					ct := models.CatatanKeuangan{UserID: profile.UserID, FileName: existingUp.FileName, Amount: amt, Date: time.Now()}
					if err := db.Create(&ct).Error; err == nil {
						existingUp.KeuanganID = &ct.ID
						db.Save(&existingUp)
					}
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"id": existingUp.ID, "path": relPath, "store_path": existingUp.StorePath, "catatan_id": existingUp.KeuanganID})
		return
	}

	up := models.Upload{ProfileID: profile.ID, FileName: file.Filename, StorePath: storePath, KeuanganID: keuID, ContentType: ct}
	if err := db.Create(&up).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db save failed"})
		return
	}
	// extract amount using OCR and avoid creating duplicate Catatan
	if amt, conf, found, err := ocr.ExtractAmountFromImage(fullPath); err == nil && amt > 0 && conf > 0.15 {
		if found != "" {
			lf := strings.TrimSpace(found)
			if strings.Contains(lf, ".") || strings.HasSuffix(lf, ",00") || strings.HasSuffix(lf, ".00") {
				if amt%100 == 0 {
					amt = amt / 100
				}
			}
		}
		var existingCat models.CatatanKeuangan
		if err := db.Where("user_id = ? AND file_name = ?", profile.UserID, up.FileName).First(&existingCat).Error; err == nil {
			up.KeuanganID = &existingCat.ID
			db.Save(&up)
		} else {
			ct := models.CatatanKeuangan{UserID: profile.UserID, FileName: up.FileName, Amount: amt, Date: time.Now()}
			if err := db.Create(&ct).Error; err == nil {
				up.KeuanganID = &ct.ID
				db.Save(&up)
			}
		}
	}
	respCatID := up.KeuanganID
	if catatanID != nil {
		respCatID = catatanID
	}
	c.JSON(http.StatusOK, gin.H{"id": up.ID, "path": relPath, "store_path": storePath, "catatan_id": respCatID})
}

// listUploadsHandler returns uploads; admin sees all, user only own profile's uploads.
func listUploadsHandler(c *gin.Context) {
	role, _ := c.Get("role")
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var profile models.Profile
	db.Where("user_id = ?", user.ID).First(&profile)
	var uploads []models.Upload
	q := db.Model(&models.Upload{})
	if role != "administrator" {
		q = q.Where("profile_id = ?", profile.ID)
	}
	if err := q.Order("id desc").Limit(100).Find(&uploads).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	c.JSON(http.StatusOK, uploads)
}

// getUploadHandler returns single upload if admin or owner.
func getUploadHandler(c *gin.Context) {
	role, _ := c.Get("role")
	user, ok := getUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var profile models.Profile
	db.Where("user_id = ?", user.ID).First(&profile)
	id := c.Param("id")
	var up models.Upload
	if err := db.First(&up, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if role != "administrator" && up.ProfileID != profile.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, up)
}
