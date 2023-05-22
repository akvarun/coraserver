package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/deebakkarthi/coraserver/db"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

// Global OAuth Configuration variable
var oauthConfig *oauth2.Config

const (
	configFile = "./config.json"
	port       = ":42069"
)

/*
Temporary struct to unmarshall the config data from config.json
We cannot put the values of ClientSecret in the src code. It is a privileged
piece of information. So the configuration is stored in a =config.json= file.
In order to enforce the correct types this struct is needed. On a side note,
notice that the fields start with an uppercase. This means that they are to be
exported(accessible outside this package). You may think that it is not going
to be used outside this package, that is true, but since we are adding that
json tag they will be used by the =encoding/json= package to deserialize. So
whenever you want to deserialize a json file the corresponding struct members
should always be exported.
*/
type oauthJSONRepr struct {
	ClientID     string   `json:"clientID"`
	ClientSecret string   `json:"clientSecret"`
	RedirectURL  string   `json:"redirectURL"`
	Scopes       []string `json:"scopes"`
	Tenant       string   `json:"tenant"`
}

/*
=init()= is a special type of function like =main()= that is called automatically
by the go runtime. It is used to setup things that are needed before the main
function. Here we are setting up the oauthConfig variable by unmarshalling the
=config.json= file
*/
func init() {

	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("Error reading JSON file:", err)
	}

	var jsonData oauthJSONRepr
	err = json.Unmarshal(file, &jsonData)
	if err != nil {
		log.Fatal("Error unmarshalling JSON:", err)
	}

	oauthConfig = &oauth2.Config{
		ClientID:     jsonData.ClientID,
		ClientSecret: jsonData.ClientSecret,
		RedirectURL:  jsonData.RedirectURL,
		Scopes:       jsonData.Scopes,
		Endpoint:     microsoft.AzureADEndpoint(jsonData.Tenant),
	}

}

func main() {

	// Rudimentary routing setup
	router := http.NewServeMux()

	router.HandleFunc("/oauth/login", oauthLoginHandler)
	router.HandleFunc("/oauth/exchange", oauthExchangeHandler)
	router.HandleFunc("/db/freeclass", freeClassHandler)
	router.HandleFunc("/db/freeslot", freeSlotHandler)
	router.HandleFunc("/db/daytimetable", dayTimetableHandler)

	server := &http.Server{Addr: port, Handler: router}

	log.Println("Server starting on port ", port)
	log.Fatal(server.ListenAndServe())
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	randomString := make([]byte, length)
	for i := 0; i < length; i++ {
		randomString[i] = charset[rand.Intn(len(charset))]
	}
	return string(randomString)
}

func oauthLoginHandler(w http.ResponseWriter, r *http.Request) {
	state := generateRandomString(16)
	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("prompt", "select_account"))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func requestGraphAPI(accessToken string, endpoint string) ([]byte, error) {
	url := "https://graph.microsoft.com/v1.0/" + endpoint
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func oauthExchangeHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	// Exchange the authorization code for an access token
	token, err := oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Println("Error while exchanging authorization code", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	userProfileJSON, err := requestGraphAPI(token.AccessToken, "me")
	if err != nil {
		log.Println("Error getting user profile", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(err.Error()))
		return
	}
	userOrgJSON, err := requestGraphAPI(token.AccessToken, "organization")
	if err != nil {
		log.Println("Error getting user organization", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(err.Error()))
		return
	}
	// Set the response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Send the JSON response in the response body
	w.Write(userProfileJSON)
	w.Write([]byte("\n"))
	w.Write(userOrgJSON)
}

func freeClassHandler(w http.ResponseWriter, r *http.Request) {
	slotStr := r.URL.Query().Get("slot")
	day := r.URL.Query().Get("day")
	slot, err := strconv.Atoi(slotStr)
	if err != nil {
		http.Error(w, "Invalid slot value", http.StatusBadRequest)
		return
	}
	var classroom []string = db.GetFreeClass(slot, day)
	// Convert classroom to JSON
	jsonResponse, err := json.Marshal(classroom)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set the content type header to application/json
	w.Header().Set("Content-Type", "application/json")

	// Write the JSON response
	w.Write(jsonResponse)
}

func freeSlotHandler(w http.ResponseWriter, r *http.Request) {
	class := r.URL.Query().Get("class")
	day := r.URL.Query().Get("day")
	var slot []int = db.GetFreeSlot(class, day)
	// Convert classroom to JSON
	jsonResponse, err := json.Marshal(slot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set the content type header to application/json
	w.Header().Set("Content-Type", "application/json")

	// Write the JSON response
	w.Write(jsonResponse)
}

func dayTimetableHandler(w http.ResponseWriter, r *http.Request) {
	class := r.URL.Query().Get("class")
	day := r.URL.Query().Get("day")
	var subject []string = db.GetTimetableByDay(class, day)
	// Convert classroom to JSON
	jsonResponse, err := json.Marshal(subject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set the content type header to application/json
	w.Header().Set("Content-Type", "application/json")

	// Write the JSON response
	w.Write(jsonResponse)
}
