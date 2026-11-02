// Command vapid prints a new VAPID key pair for Web Push, as .env lines:
//
//	go run ./cmd/vapid >> .env
//
// Keep the private key secret. Changing the keys later makes every existing
// browser subscription stop working (customers just subscribe again).
package main

import (
	"fmt"
	"log"

	"ecommerce/backend/internal/push"
)

func main() {
	pub, priv, err := push.GenerateKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", pub, priv)
}
