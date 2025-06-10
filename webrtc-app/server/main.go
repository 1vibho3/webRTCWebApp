package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	// "time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/pion/webrtc/v3"
)

func main() {
	// Load TLS certificates
	cert, err := tls.LoadX509KeyPair("../cert/localhost.pem", "../cert/localhost-key.pem")
	if err != nil {
		log.Fatal("failed to load certs:", err)
	}

	// HTTP multiplexer and WebTransport server
	mux := http.NewServeMux()
	wtServer := &webtransport.Server{
		H3: http3.Server{
			Addr:    ":4433",
			Handler: mux,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		},
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "https://localhost:8081" || origin == ""
		},
	}

	// WebTransport /session handler
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received WebTransport request from %s", r.RemoteAddr)
		sess, err := wtServer.Upgrade(w, r)
		if err != nil {
			log.Printf("Upgrade failed: %v", err)
			http.Error(w, "upgrade failed", http.StatusInternalServerError)
			return
		}
		log.Println("WebTransport session established")
		go handleSession(sess)
	})

	// CORS root for debugging
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

	log.Println("Starting WebTransport server on https://localhost:4433")
	if err := wtServer.ListenAndServeTLS("../cert/localhost.pem", "../cert/localhost-key.pem"); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func handleSession(sess *webtransport.Session) {
	ctx := context.Background()
	for {
		stream, err := sess.AcceptStream(ctx)
		if err != nil {
			log.Printf("AcceptStream error: %v", err)
			return
		}
		log.Println("New WebTransport stream accepted")
		go handleStream(stream)
	}
}

func handleStream(stream webtransport.Stream) {
	defer stream.Close()

	// 1) Read incoming SDP offer JSON
	buf := make([]byte, 64*1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("Read error: %v", err)
		return
	}

	var offer webrtc.SessionDescription

	if err := json.Unmarshal(buf[:n], &offer); err != nil {
		log.Printf("Invalid SDP offer: %v", err)
		return
	}
	log.Printf("Received SDP offer:\n%s", offer.SDP)

	// 2) Create Pion PeerConnection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err := logError("PeerConnection error", err); err != nil {
		return
	}
	defer pc.Close()

	// 3) Set remote description
	if err := logError("SetRemoteDescription error", pc.SetRemoteDescription(offer)); err != nil {
		return
	}

	// 4) Create and set local SDP answer
	answer, err := pc.CreateAnswer(nil)
	log.Printf("Sending SDP answer:\n%s", answer.SDP)

	if err := logError("CreateAnswer error", err); err != nil {
		return
	}
	if err := logError("SetLocalDescription error", pc.SetLocalDescription(answer)); err != nil {
		return
	}

	// 5) Wait for ICE gathering
	<-webrtc.GatheringCompletePromise(pc)

	// 6) Send SDP answer back over WebTransport
	answerBytes, err := json.Marshal(pc.LocalDescription())
	if err := logError("Marshal answer error", err); err != nil {
		return
	}
	if _, err := stream.Write(answerBytes); err != nil {
		log.Printf("Write answer error: %v", err)
		return
	}
}

// helper to log and wrap errors
func logError(prefix string, err error) error {
	if err != nil {
		log.Printf("%s: %v", prefix, err)
		return err
	}
	return nil
}
