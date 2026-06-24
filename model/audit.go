package model

// AuditLog is an append-only record of every security-relevant action. It is
// never updated or deleted. CRITICAL: Detail must NEVER contain key plaintext —
// callers pass only non-secret context (ids, names, fingerprints, outcomes).
type AuditLog struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	UserId       int    `json:"user_id" gorm:"index"`
	Username     string `json:"username" gorm:"type:varchar(50)"`
	Action       string `json:"action" gorm:"type:varchar(50);index"` // e.g. CREATE_CHANNEL, SYNC_OK, LOGIN
	ResourceType string `json:"resource_type" gorm:"type:varchar(30)"`
	ResourceId   int    `json:"resource_id" gorm:"index"`
	Detail       string `json:"detail" gorm:"type:text"` // non-secret JSON only
	Ip           string `json:"ip" gorm:"type:varchar(45)"`
	Result       string `json:"result" gorm:"type:varchar(20)"` // success | failed
	CreatedTime  int64  `json:"created_time" gorm:"index"`
}

// WriteAudit appends an audit record. Best-effort: a logging failure must not
// break the primary operation, so callers typically ignore the returned error
// (it is surfaced for tests).
func WriteAudit(e *AuditLog) error {
	e.CreatedTime = nowUnix()
	return DB.Create(e).Error
}

// ListAudit returns audit entries filtered by optional action/user, newest first.
func ListAudit(action string, userId, offset, limit int) ([]AuditLog, int64, error) {
	q := DB.Model(&AuditLog{})
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []AuditLog
	err := q.Order("id desc").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}
