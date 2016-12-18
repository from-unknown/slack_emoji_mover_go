package main

import (
	"bufio"
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/headzoo/surf"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

func main() {
	// Variables
	var slack_url string
	var email string
	var password string
	var token string

	const loginFail string = "Sorry, you entered an incorrect email address or password."
	const addSuccess string = "Your new emoji has been saved"
	f, err := os.OpenFile("emoji.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	log.SetOutput(f)

	// Slack API url
	slack_api := "https://slack.com/api/emoji.list?pretty=1&token="

	type EmojiList struct {
		Name, Url string
	}

	log.Println("Reading emoji_conf.txt...")
	// Check ini file existance
	file, err := os.Open("./emoji_conf.txt")
	if err != nil {
		log.Fatal("Could not load emoji_conf.txt file.")
	}
	defer file.Close()

	// Read conf file
	confList := list.New()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		tmp := scanner.Text()
		tmp = strings.TrimSpace(tmp)
		if tmp != "" && string(tmp[0:1]) != "#" {
			confList.PushBack(tmp)
		}
	}
	// confList size must be greater than 4
	if confList.Len() < 4 {
		log.Fatal("Config file doesn't have enough settings.")
	}
	// Set conf data
	slack_url = confList.Remove(confList.Front()).(string)
	email = confList.Remove(confList.Front()).(string)
	password = confList.Remove(confList.Front()).(string)
	token = confList.Remove(confList.Front()).(string)

	log.Println("Reading default_emoji.txt...")
	// Check default emoji file exists
	file, err = os.Open("./default_emoji.txt")
	if err != nil {
		log.Fatal("Could not load default_emoji.txt file.")
	}
	defer file.Close()

	// Read default emoji file
	defaultEmojiList := list.New()
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		tmp := scanner.Text()
		tmp = strings.TrimSpace(tmp)
		defaultEmojiList.PushBack(tmp)
	}

	log.Println("Accessing to slack API to get emoji list...")
	// Get emoji file from slack team
	resp, err := http.Get(slack_api + token)
	if err != nil {
		// handle error
		log.Fatal("Could not access slack api.")
	}
	defer resp.Body.Close()

	// Read JSON data
	dec := json.NewDecoder(resp.Body)
	var jsonDataMap interface{}
	for dec.More() {
		var data interface{}
		if err := dec.Decode(&data); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Error while reading JSON data.")
		}
		jsonDataMap = data.(map[string]interface{})["emoji"]
	}

	log.Println("Opening slack team page...")
	// Create a new browser and open Slack team
	bow := surf.NewBrowser()
	err = bow.Open(slack_url)
	if err != nil {
		log.Fatal("Could not access slack team.")
	}

	log.Println("Trying to sign in...")
	// Log in to the site
	fm, _ := bow.Form("form#signin_form")
	if err != nil {
		log.Fatal("Could not access signin form.")
	}

	fm.Input("email", email)
	fm.Input("password", password)
	if fm.Submit() != nil {
		log.Fatal("Could not sign in to slack team.")
	}

	// Check login success or failed
	bow.Find("p.alert_error").Each(func(_ int, s *goquery.Selection) {
		tmpStr := strings.TrimSpace(s.Text())
		if tmpStr == loginFail {
			log.Fatal("Could not sign in to slack team.\nPlease check email and password.")
		}
	})

	log.Println("Accessing to customize page...")
	// Open customize/emoji page
	err = bow.Open(slack_url + "customize/emoji")
	if err != nil {
		log.Fatal("Could not access slack customize emoji page.")
	}

	// Find registered emoji from webpage
	existList := list.New()
	bow.Find("td.align_middle").Each(func(_ int, s *goquery.Selection) {
		// emoji name are formatted :xxx: from, so use regexp to check
		match, err := regexp.MatchString(":*:", s.Text())
		if err != nil {
			log.Fatal("Error while checking web page.")
		}
		if match {
			tmpStr := strings.TrimSpace(s.Text())
			tmpStr = tmpStr[1 : utf8.RuneCountInString(tmpStr)-1]
			existList.PushBack(tmpStr)
		}
	})

	// Copy keys
	keys := make([]string, len(jsonDataMap.(map[string]interface{})))

	i := 0
	for tmpKey := range jsonDataMap.(map[string]interface{}) {
		keys[i] = tmpKey
		i++
	}

	// Delete default emoji and emoji already exists
	for _, val := range keys {
		if includeInList(defaultEmojiList, val) {
			delete(jsonDataMap.(map[string]interface{}), val)
		} else if includeInList(existList, val) {
			delete(jsonDataMap.(map[string]interface{}), val)
		}
	}

	// Download emoji image from url
	for k, v := range jsonDataMap.(map[string]interface{}) {
		if v.(string) != "" && string(v.(string)[0:5]) != "alias" {
			result, exist := downImageFile(v.(string))
			if result {
				if exist {
					log.Println(k + " already exists...")
				} else {
					log.Println(k + " downloaded...")
					time.Sleep(3 * time.Second)
				}
			} else {
				log.Println("Error while downloading " + k + ".")
			}
		} else {
			delete(jsonDataMap.(map[string]interface{}), k)
		}
	}

	log.Println("Uploading emoji...")
	// Upload emoji
	for k, v := range jsonDataMap.(map[string]interface{}) {
		fm, _ = bow.Form("form#addemoji")
		if err != nil {
			log.Println("Error while finding emoji form.")
		}

		fm.Input("name", k)
		_, filename := path.Split(v.(string))
		read, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Println("Error while opening emoji (" + k + ") file.")
		}
		fm.File("img", filename, bytes.NewBuffer(read))
		fm.Input("mode", "data")
		if fm.Submit() != nil {
			log.Println("Error while submitting emoji.")
		}
		bow.Find("p.alert_success").Each(func(_ int, s *goquery.Selection) {
			tmpStr := strings.TrimSpace(s.Text())
			if err != nil {
				log.Fatal("Error while checking web page.")
			}
			if strings.Index(tmpStr, addSuccess) > -1 {
				log.Println(k + " successfully added.")
			} else {
				log.Println(k + " could not added.")
			}
		})

		err = bow.Open(slack_url + "customize/emoji")
		if err != nil {
			log.Fatal("Could not access slack customize emoji page.")
		}
		time.Sleep(5 * time.Second)
	}

}

// Download image file
// return bool : true  file successfully added
//               false could not downloaded
//        bool : true  file already exists
//               false file did not exists
func downImageFile(url string) (bool, bool) {
	_, filename := path.Split(url)

	// file existance check
	_, err := os.Stat(filename)
	if err == nil {
		return true, true
	}

	response, err := http.Get(url)

	if err != nil {
		fmt.Println(err)
		return false, false
	}

	body, err := ioutil.ReadAll(response.Body)

	if err != nil {
		fmt.Println(err)
		return false, false
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)

	if err != nil {
		fmt.Println(err)
		return false, false
	}

	defer func() {
		file.Close()
	}()

	file.Write(body)
	return true, false
}

// Check if target is in the list or not.
func includeInList(l *list.List, target string) bool {
	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value == target {
			return true
		}
	}
	return false
}
