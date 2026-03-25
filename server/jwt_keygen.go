package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func jwtKeygen(args []string) {
	fs := flag.NewFlagSet("jwt-keygen", flag.ExitOnError)
	privatePath := fs.String("private", "", "private key output path (required)")
	publicPath := fs.String("public", "", "public key output path (required)")
	bits := fs.Int("bits", 2048, "rsa key size")
	overwrite := fs.Bool("overwrite", false, "overwrite existing files")
	_ = fs.Parse(args)

	priv := strings.TrimSpace(*privatePath)
	pub := strings.TrimSpace(*publicPath)
	if priv == "" || pub == "" {
		log.Fatal("both --private and --public are required")
	}
	if !*overwrite {
		if _, err := os.Stat(priv); err == nil {
			log.Fatalf("private key file already exists: %s", priv)
		}
		if _, err := os.Stat(pub); err == nil {
			log.Fatalf("public key file already exists: %s", pub)
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		log.Fatalf("unable to generate rsa key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		log.Fatalf("unable to encode public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})
	if err := os.MkdirAll(filepath.Dir(priv), 0o755); err != nil {
		log.Fatalf("unable to create private key directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(pub), 0o755); err != nil {
		log.Fatalf("unable to create public key directory: %v", err)
	}
	if err := os.WriteFile(priv, privPEM, 0o600); err != nil {
		log.Fatalf("unable to write private key: %v", err)
	}
	if err := os.WriteFile(pub, pubPEM, 0o644); err != nil {
		log.Fatalf("unable to write public key: %v", err)
	}
	log.Printf("generated jwt keypair (private=%s public=%s bits=%d)", priv, pub, *bits)
}
