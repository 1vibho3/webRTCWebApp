package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

func main() {
	// Load TLS certificates
	cert, err := tls.LoadX509KeyPair("../cert/localhost.pem", "../cert/localhost-key.pem")
	if err != nil {
		log.Fatal("failed to load certs:", err)
	}

	// Create HTTP multiplexer
	mux := http.NewServeMux()

	// Initialize WebTransport server with HTTP/3 config
	wtServer := &webtransport.Server{
    H3: http3.Server{
        Addr:    ":4433",
        Handler: mux,
        TLSConfig: &tls.Config{
            Certificates: []tls.Certificate{cert},
        },
    },
    CheckOrigin: func(r *http.Request) bool {
        // Allow connections from your HTTPS client
        origin := r.Header.Get("Origin")
        return origin == "https://localhost:8081" || origin == ""
    },
}

	// Route for WebTransport sessions
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request from %s", r.RemoteAddr)
		
		sess, err := wtServer.Upgrade(w, r)
		if err != nil {
			log.Printf("Upgrade failed: %v", err)
			http.Error(w, "upgrade failed", http.StatusInternalServerError)
			return
		}
		
		log.Println("WebTransport session established")
		go handleSession(sess)
	})

	// Add CORS headers for browser testing
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		fmt.Fprintf(w, "WebTransport server is running. Connect to /session")
	})

	// Start the server (WebTransport + HTTP/3)
	log.Println("Starting WebTransport server on https://localhost:4433")
	log.Println("Connect to: https://localhost:4433/session")
	
	if err := wtServer.ListenAndServeTLS("../cert/localhost.pem", "../cert/localhost-key.pem"); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func handleSession(sess *webtransport.Session) {
	ctx := context.Background()
	log.Println("Session handler started")

	// Handle bidirectional streams
	for {
		stream, err := sess.AcceptStream(ctx)
		if err != nil {
			log.Printf("AcceptStream error: %v", err)
			return
		}
		
		log.Println("New stream accepted")
		go handleStream(stream)
	}
}

func handleStream(stream webtransport.Stream) {
	defer stream.Close()
	
	for {
		// Read data from stream
		buffer := make([]byte, 1024)
		n, err := stream.Read(buffer)
		if err != nil {
			if err == io.EOF {
				log.Println("Stream closed by client")
				return
			}
			log.Printf("Read error: %v", err)
			return
		}
		
		message := string(buffer[:n])
		log.Printf("Received: %s", message)
		
		// Echo back with JSON response
		response := map[string]interface{}{
			"echo":      message,
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
			"length":    n,
		}
		
		if err := json.NewEncoder(stream).Encode(response); err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}