// Command agt-vault is the secure multi-supplier API-key custody platform.
//
// It boots in this order, failing fast on any misconfiguration:
//  1. Load + validate config (MASTER_KEK, SESSION_SECRET required).
//  2. Initialize the crypto vault from MASTER_KEK (in memory only).
//  3. Open + migrate the database.
//  4. Seed the initial root account if the user table is empty.
//  5. Start the HTTP server.
package main

import (
	"fmt"
	"log"

	"github.com/modex/agt-vault/config"
	"github.com/modex/agt-vault/crypto"
	"github.com/modex/agt-vault/model"
	"github.com/modex/agt-vault/router"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	// Vault first — nothing secret-related works without it.
	if err := crypto.InitGlobal(cfg.MasterKEKHex); err != nil {
		log.Fatalf("failed to initialize crypto vault: %v", err)
	}
	log.Println("crypto vault initialized")

	if err := model.Init(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	log.Println("database initialized")

	created, generatedPw, err := model.SeedRootUser(cfg.RootUsername, cfg.RootPassword)
	if err != nil {
		log.Fatalf("failed to seed root user: %v", err)
	}
	if created {
		if generatedPw != "" {
			log.Printf("================ INITIAL ADMIN ACCOUNT ================")
			log.Printf("  username: %s", cfg.RootUsername)
			log.Printf("  password: %s", generatedPw)
			log.Printf("  (generated; set ROOT_PASSWORD to choose your own)")
			log.Printf("  CHANGE THIS PASSWORD AFTER FIRST LOGIN")
			log.Printf("======================================================")
		} else {
			log.Printf("seeded initial admin account %q", cfg.RootUsername)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	r := router.Setup(cfg.SessionSecret)
	router.MountSPA(r, distFS())
	if cfg.TrustedProxies != "" {
		_ = r.SetTrustedProxies([]string{cfg.TrustedProxies})
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("agt-vault listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
