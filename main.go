package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"io"
	"net/http"

	"time"

	"os/exec"

	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/pkg/browser"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

func markdownFileToHTML(filename string) []byte {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	return blackfriday.Run(bytes, blackfriday.WithExtensions(blackfriday.Tables|blackfriday.FencedCode))
}

var template = `
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8"/>
    <title>Document</title>
  </head>
  <body>
    <div id="content"></div>
    <script>
     window.onload = function () {
         if (!window["WebSocket"]) {
             alert("エラー : WebSocketに対応していないブラウザです。")
         } else {
             socket = new WebSocket("ws://localhost:1129/ws");
             socket.onclose = function() {
                 alert(" 接続が終了しました。");
             }
             socket.onmessage = function(e) {
                 var content = document.getElementById('content');
                 content.innerHTML = e.data;
             }
         }
     };
    </script>
  </body>
</html>
`

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func main() {
	var err error

	filename := os.Args[1]

	result := markdownFileToHTML(filename)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err = io.WriteString(w, template)
		if err != nil {
			panic(err)
		}
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	err = watcher.Add(filename)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			if _, ok := err.(websocket.HandshakeError); !ok {
				log.Println(err)
			}
			return
		}

		result = markdownFileToHTML(filename)
		ws.WriteMessage(websocket.TextMessage, result)

		go func() {
			for {
				select {
				case event := <-watcher.Events:
					result = markdownFileToHTML(filename)
					ws.WriteMessage(websocket.TextMessage, result)
					log.Println("event:", event)
					if event.Op&fsnotify.Write == fsnotify.Write {
						log.Println("modified file:", event.Name)
					}
				case err := <-watcher.Errors:
					log.Println("error:", err)
				}
			}
		}()
		for {
			time.Sleep(1 * time.Second)
		}
	})

	go func() {
		if err = http.ListenAndServe(":1129", nil); err != nil {
			panic(err)
		}
	}()

	go func() {
		if err = browser.OpenURL("http://localhost:1129"); err != nil {
			panic(err)
		}
	}()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		log.Println("Set $EDITOR")
		os.Exit(1)
	}

	splitted := strings.Split(editor, " ")
	log.Printf("splitted: %#v\n", splitted)
	cname := splitted[0]
	args := splitted[1:]
	args = append(args, filename)

	cmd := exec.Command(cname, args[:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	if err = cmd.Run(); err != nil {
		log.Printf("editor error: %v\n", err)
		os.Exit(1)
	}
}
