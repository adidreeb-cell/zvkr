package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"exeldoctor/internal/models"
	"exeldoctor/internal/services/auth"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func generatePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		password[i] = charset[randomIndex.Int64()]
	}

	return string(password), nil
}

type AuthHandler struct {
	JWTSecret     []byte
	DB            gorm.DB
	SetupPassword string
}

type Credentials struct {
	Username string          `json:"username"`
	Password string          `json:"password"`
	Role     models.RoleType `json:"role"` // Используется только при регистрации
}

// GetSetupInfo - отдает пароль, если это первый запуск
func (h *AuthHandler) GetSetupInfo(c *fiber.Ctx) error {
	if h.SetupPassword == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Система уже инициализирована или доступ запрещен",
		})
	}

	return c.JSON(fiber.Map{
		"username": "admin",
		"password": h.SetupPassword,
	})
}

// CompleteSetup - зачищает пароль из памяти навсегда
func (h *AuthHandler) CompleteSetup(c *fiber.Ctx) error {
	h.SetupPassword = ""
	return c.JSON(fiber.Map{"success": true})
}

// Login - авторизация
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var creds Credentials

	// Парсим JSON из тела запроса
	if err := c.BodyParser(&creds); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Неверный формат запроса",
		})
	}

	var user models.User
	// Ищем пользователя в БД
	if err := h.DB.Where("username = ?", creds.Username).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Пользователь не найден",
		})
	}

	// Проверяем пароль
	if !auth.CheckPassword(creds.Password, user.PasswordHash) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Неверный пароль",
		})
	}

	// Генерируем JWT
	token, err := auth.GenerateToken(user.ID, user.Role, h.JWTSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка генерации токена",
		})
	}

	// Отдаем успешный ответ
	return c.JSON(fiber.Map{
		"token": token,
		"role":  user.Role,
	})
}

// InitAdminUser создает admin пользователя только если он не существует.
// Возвращает (пароль, true, nil) если пользователь создан,
// или ("", false, nil) если уже существует.
func (h *AuthHandler) InitAdminUser() (string, bool, error) {
	var existing models.User
	if err := h.DB.Where("username = ?", "admin").First(&existing).Error; err == nil {
		return "", false, nil
	}

	password, _ := generatePassword(32)
	hashedPassword, _ := auth.HashPassword(password)

	user := models.User{
		Username:     "admin",
		PasswordHash: hashedPassword,
		Role:         models.Admin,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return "", false, err
	}

	return password, true, nil
}

// Register - регистрация
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var creds Credentials

	if err := c.BodyParser(&creds); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Неверный формат запроса",
		})
	}

	hashedPassword, err := auth.HashPassword(creds.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка сервера при хэшировании",
		})
	}

	var role models.RoleType = "user"
	if creds.Role != "" {
		role = creds.Role
	}

	user := models.User{
		Username:     creds.Username,
		PasswordHash: hashedPassword,
		Role:         role,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Пользователь с таким именем уже существует",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Пользователь успешно создан",
	})
}

type CreateUserRequest struct {
	Username string          `json:"username"`
	Role     models.RoleType `json:"role"`
}

func (h *AuthHandler) CreateUser(c *fiber.Ctx) error {

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Неверный формат запроса"})
	}

	if req.Username == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Имя пользователя обязательно"})
	}

	if req.Role == "" {
		req.Role = models.UserRole
	}

	password, err := generatePassword(32)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Ошибка генерации пароля"})
	}

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Ошибка хэширования пароля"})
	}

	user := models.User{
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Role:         req.Role,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Пользователь уже существует"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"username": user.Username,
		"role":     user.Role,
		"password": password,
	})
}

func (h *AuthHandler) GetUsers(c *fiber.Ctx) error {
	var users []models.User
	if err := h.DB.Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Ошибка получения пользователей"})
	}

	type UserResponse struct {
		ID        uint            `json:"id"`
		Username  string          `json:"username"`
		Role      models.RoleType `json:"role"`
		CreatedAt string          `json:"created_at"`
		UpdatedAt string          `json:"updated_at"`
	}

	response := make([]UserResponse, len(users))
	for i, u := range users {
		response[i] = UserResponse{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: u.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return c.JSON(response)
}

func (h *AuthHandler) DeleteUser(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(float64)
	targetID := c.Params("id")

	if fmt.Sprint(uint(userID)) == targetID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Нельзя удалить самого себя"})
	}

	var user models.User
	if err := h.DB.First(&user, targetID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Пользователь не найден"})
	}

	if err := h.DB.Delete(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Ошибка удаления пользователя"})
	}

	return c.JSON(fiber.Map{"message": "Пользователь успешно удален"})
}
