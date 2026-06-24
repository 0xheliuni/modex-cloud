package model

import (
	"errors"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"gorm.io/gorm"
)

// User is both an admin and a supplier account (role-discriminated). Under the
// "one-supplier-one-account" decision there is no separate Supplier table; a
// supplier's identity (code/name) lives directly on the User row.
type User struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	Username     string `json:"username" gorm:"uniqueIndex;type:varchar(50);not null"`
	Password     string `json:"-" gorm:"type:varchar(255);not null"` // bcrypt; never serialized
	Role         int    `json:"role" gorm:"default:1;index"`
	Status       int    `json:"status" gorm:"default:1"`
	SupplierCode string `json:"supplier_code" gorm:"type:varchar(20);index"` // e.g. "11"
	SupplierName string `json:"supplier_name" gorm:"type:varchar(100)"`      // e.g. "Modex"

	// AccessToken authenticates API (non-session) calls. Pointer so the unique
	// index tolerates multiple NULLs (accounts without a token).
	AccessToken *string `json:"-" gorm:"uniqueIndex;type:varchar(64)"`

	// Two-factor (TOTP). Secret is sealed by the crypto vault before storage.
	TwoFAEnabled   bool   `json:"two_fa_enabled" gorm:"default:false"`
	TwoFASecretEnc string `json:"-" gorm:"type:text"`

	CreatedTime   int64  `json:"created_time"`
	LastLoginTime int64  `json:"last_login_time"`
	LastLoginIp   string `json:"last_login_ip" gorm:"type:varchar(45)"`
}

var ErrUserNotFound = errors.New("user not found")

// IsAdmin reports whether the user holds at least the admin role.
func (u *User) IsAdmin() bool { return u.Role >= constant.RoleAdmin }

// IsSupplier reports whether the user is exactly a supplier.
func (u *User) IsSupplier() bool { return u.Role == constant.RoleSupplier }

// Create inserts a new user, hashing the supplied plaintext password.
func (u *User) Create(plainPassword string) error {
	hashed, err := common.Password2Hash(plainPassword)
	if err != nil {
		return err
	}
	u.Password = hashed
	u.CreatedTime = nowUnix()
	if u.Status == 0 {
		u.Status = constant.StatusEnabled
	}
	return DB.Create(u).Error
}

// GetUserById loads a user by primary key.
func GetUserById(id int) (*User, error) {
	if id == 0 {
		return nil, ErrUserNotFound
	}
	var u User
	err := DB.First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &u, err
}

// GetUserByUsername loads a user by unique username.
func GetUserByUsername(username string) (*User, error) {
	var u User
	err := DB.First(&u, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &u, err
}

// ValidateAccessToken resolves an API access token to its (enabled) user.
func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrUserNotFound
	}
	var u User
	err := DB.First(&u, "access_token = ?", token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	return &u, err
}

// UpdatePassword sets a new bcrypt-hashed password.
func (u *User) UpdatePassword(plainPassword string) error {
	hashed, err := common.Password2Hash(plainPassword)
	if err != nil {
		return err
	}
	return DB.Model(u).Update("password", hashed).Error
}

// TouchLogin records a successful login's time and source IP.
func TouchLogin(userId int, ip string) error {
	return DB.Model(&User{}).Where("id = ?", userId).Updates(map[string]any{
		"last_login_time": nowUnix(),
		"last_login_ip":   ip,
	}).Error
}

// ListUsers returns users filtered by optional role, newest first. Passwords and
// tokens never serialize (json:"-").
func ListUsers(role, offset, limit int) ([]User, int64, error) {
	q := DB.Model(&User{})
	if role > 0 {
		q = q.Where("role = ?", role)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var us []User
	err := q.Order("id desc").Offset(offset).Limit(limit).Find(&us).Error
	return us, total, err
}

// UpdateProfile writes mutable, non-secret user fields (admin edit).
func (u *User) UpdateProfile() error {
	return DB.Model(u).Updates(map[string]any{
		"role":          u.Role,
		"status":        u.Status,
		"supplier_code": u.SupplierCode,
		"supplier_name": u.SupplierName,
	}).Error
}

// SetAccessToken assigns (or rotates) the user's API access token.
func (u *User) SetAccessToken(token string) error {
	return DB.Model(u).Update("access_token", token).Error
}

// DeleteUser removes a user and cascades their grants and channels. Channels are
// hard-deleted here because, post-sync, they hold no secret anyway; pending ones
// are abandoned (their sealed key dies with the row).
func DeleteUser(id int) error {
	if err := DB.Where("user_id = ?", id).Delete(&Grant{}).Error; err != nil {
		return err
	}
	if err := DB.Where("user_id = ?", id).Delete(&Channel{}).Error; err != nil {
		return err
	}
	return DB.Delete(&User{}, "id = ?", id).Error
}

// SeedRootUser creates the initial admin/root account if no users exist yet.
// Returns (created, generatedPassword, error). If password is empty a random one
// is generated and returned so the operator can capture it from startup logs.
func SeedRootUser(username, password string) (bool, string, error) {
	var count int64
	if err := DB.Model(&User{}).Count(&count).Error; err != nil {
		return false, "", err
	}
	if count > 0 {
		return false, "", nil
	}
	generated := ""
	if password == "" {
		p, err := common.GenerateRandomCharsKey(16)
		if err != nil {
			return false, "", err
		}
		password, generated = p, p
	}
	u := &User{
		Username: username,
		Role:     constant.RoleRoot,
		Status:   constant.StatusEnabled,
	}
	if err := u.Create(password); err != nil {
		return false, "", err
	}
	return true, generated, nil
}
