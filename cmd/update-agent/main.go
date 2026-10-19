package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"link-bot/internal/updater"
)

func main() {
	repo := flag.String("repo", "", "Fixed Link-Bot checkout")
	dir := flag.String("dir", "", "Private updater directory")
	flag.Parse()
	token, err := os.ReadFile(filepath.Join(*dir, "token"))
	if err != nil {
		log.Fatal(err)
	}
	agent, err := updater.NewAgent(*repo, *dir, strings.TrimSpace(string(token)))
	if err != nil {
		log.Fatal(err)
	}
	socket := filepath.Join(*dir, "agent.sock")
	// A second service must not unlink the socket of a running instance.
	if conn, err := net.DialTimeout("unix", socket, time.Second); err == nil {
		conn.Close()
		log.Fatal("update agent already running")
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0666); err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: agent, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4096}
	log.Fatal(server.Serve(listener))
}
