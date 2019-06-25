package main

import (
	"bufio"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	//TODO read it from .env
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)

	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := gmail.New(client)
	fmt.Println("errr: ", err)
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}

	watchReq := &gmail.WatchRequest{}
	watchReq.LabelIds = []string{"INBOX"}
	watchReq.TopicName = "gmailtopic"
	c := srv.Users.Watch("me", watchReq)

	fmt.Println("watch: ", c)
	user := "me"
	r, err := srv.Users.Labels.List(user).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve labels: %v", err)
	}
	if len(r.Labels) == 0 {
		fmt.Println("No labels found.")
		return
	}

	// m, err := srv.Users.Messages.List(user).Do()

	// if err != nil {
	// 	log.Fatal(err)
	// }

	// for _, v := range m.Messages {
	// 	fmt.Printf("- %s\n ")
	// }

	fmt.Println("Labels:")
	for _, l := range r.Labels {
		fmt.Printf(" %s\n", l)
	}

	//   go-api-demo -clientid="my-clientid" -secret="my-secret" gmail

	// argv := []string{}
	gmailMain(client)
}

type message struct {
	size    int64
	gmailID string
	date    string // retrieved from message header
	snippet string
	payload string
}

func gmailMain(client *http.Client) {
	// if len(argv) != 0 {
	// 	fmt.Fprintln(os.Stderr, "Usage: gmail")
	// 	return
	// }

	svc, err := gmail.New(client)
	if err != nil {
		log.Fatalf("Unable to create Gmail service: %v", err)
	}

	var total int64
	msgs := []message{}
	pageToken := ""
	for {
		req := svc.Users.Messages.List("me").Q("after:2019/03/12 in:inbox -category:{social promotions forums}")
		if pageToken != "" {
			req.PageToken(pageToken)
		}
		r, err := req.Do()
		if err != nil {
			log.Fatalf("Unable to retrieve messages: %v", err)
		}

		log.Printf("Processing %v messages...\n", len(r.Messages))
		for _, m := range r.Messages {
			msg, err := svc.Users.Messages.Get("me", m.Id).Do()
			if err != nil {
				log.Fatalf("Unable to retrieve message %v: %v", m.Id, err)
			}
			total += msg.SizeEstimate
			date := ""
			for _, h := range msg.Payload.Headers {
				if h.Name == "Date" {
					date = h.Value
					break
				}
			}

			// fmt.Println(msg.Payload.Parts[0].Body.Data)
			//  var b []byte
			sEnc, _ := b64.StdEncoding.DecodeString(msg.Payload.Parts[0].Body.Data)

			msgs = append(msgs, message{
				size:    msg.SizeEstimate,
				gmailID: msg.Id,
				date:    date,
				snippet: msg.Snippet,
				payload: string(sEnc),
			})
		}

		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken
	}
	log.Printf("total: %v\n", total)

	sortBySize(msgs)
	reader := bufio.NewReader(os.Stdin)
	count, deleted := 0, 0
	for _, m := range msgs {
		count++
		fmt.Printf("\nMessage URL: https://mail.google.com/mail/u/0/#all/%v\n", m.gmailID)
		// fmt.Printf("Size: %v, Date: %v, Snippet: %q\n", m.size, m.date, m.snippet)
		fmt.Printf("Size: %v, Date: %v\n", m.size, m.date)
		fmt.Println(m.payload)
		fmt.Printf("Options: (d)elete, (s)kip, (q)uit: [s] ")
		val := ""
		if val, err = reader.ReadString('\n'); err != nil {
			log.Fatalf("unable to scan input: %v", err)
		}
		val = strings.TrimSpace(val)
		switch val {
		case "d": // delete message
			if err := svc.Users.Messages.Delete("me", m.gmailID).Do(); err != nil {
				log.Fatalf("unable to delete message %v: %v", m.gmailID, err)
			}
			log.Printf("Deleted message %v.\n", m.gmailID)
			deleted++
		case "q": // quit
			log.Printf("Done.  %v messages processed, %v deleted\n", count, deleted)
			os.Exit(0)
		default:
		}
	}
}

type messageSorter struct {
	msg  []message
	less func(i, j message) bool
}

func sortBySize(msg []message) {
	sort.Sort(messageSorter{msg, func(i, j message) bool {
		return i.size > j.size
	}})
}

func (s messageSorter) Len() int {
	return len(s.msg)
}

func (s messageSorter) Swap(i, j int) {
	s.msg[i], s.msg[j] = s.msg[j], s.msg[i]
}

func (s messageSorter) Less(i, j int) bool {
	return s.less(s.msg[i], s.msg[j])
}
