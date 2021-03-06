package main

import (
	"encoding/json"
	"fmt"
	"github.com/tkanos/gonfig"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"html/template"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
	"golang.org/x/net/context"
	"errors"
)

var conf oauth2.Config
const APP_ID_CONFIG = "/config/appid_config.json"
const OPEN_ID_SCOPE = "openid"
const PROFILE_SCOPE = "profile"
const STATE = "state"
const SESSION_TOKEN ="session_token"

// Home struct, used for home.html template
type Home struct{
	Title string
	User User
}

// User struct, holds all the user info shown in home.html
type User struct {
	Token string
	Profile string
}

// App ID configuration struct
type AppIdConfiguration struct {
	ClientId string
	ClientSecret string
	AuthURL string
	RedirectUrl string
}

// Builds a configuration object, with a given appidConfiguration struct
func buildConfigurationObject(app_id_configuration AppIdConfiguration) oauth2.Config {

	log.Println("Building configuration file.")

	conf := &oauth2.Config{
		ClientID: app_id_configuration.ClientId,
		ClientSecret: app_id_configuration.ClientSecret,
		RedirectURL: app_id_configuration.RedirectUrl,
		Scopes: []string{OPEN_ID_SCOPE, PROFILE_SCOPE},
		Endpoint:oauth2.Endpoint{
			AuthURL: app_id_configuration.AuthURL+"/authorization",
			TokenURL:  app_id_configuration.AuthURL+"/token",

		},
	}
	return *conf
}

// Loads a configuration file, found in /config/appid_config.json
func loadConfigurationFile() (AppIdConfiguration, error){

	log.Println("Loading configuration file.")

	app_id_configuration := AppIdConfiguration{}

	// Using runtime.Caller, to make sure we get the path where the program is being executed
	_, filename, _, ok := runtime.Caller(0)

	if !ok{
		return app_id_configuration, errors.New("Error calling runtime caller.")
	}

	// Reading configuration file
	app_id_configuration_error := gonfig.GetConf(path.Dir(filename)+string(os.PathSeparator)+APP_ID_CONFIG, &app_id_configuration)

	if app_id_configuration_error != nil {
		return app_id_configuration, app_id_configuration_error
	}

	return app_id_configuration, nil
}

// Requests an OAuthToken using a "code" type
func GetOauthToken(r *http.Request) (*oauth2.Token, error){

	log.Println("Getting auth token.")

	ctx := context.Background()

	if ctx == nil{
		return nil, errors.New("Could not get context.")
	}

	if r.URL.Query().Get(STATE) != STATE {
		return nil, errors.New("State value did not match.")
	}

	// Exchange code for OAuth token
	oauth2Token, oauth2TokenError := conf.Exchange(ctx, r.URL.Query().Get("code"))
	if oauth2TokenError != nil {
		return nil, errors.New("Failed to exchange token:"+ oauth2TokenError.Error())
	}

	return oauth2Token, nil
}

// Requests a user profile, using a bearer token
func GetUserProfile(r *http.Request, token oauth2.Token) (interface{}, error){

	log.Println("Getting user profile.")

	ctx := context.Background()

	if ctx == nil{
		return nil, errors.New("Could not get context.")
	}

	// Getting now the userInfo
	client := conf.Client(ctx, &token)

	// Get request using /userinfo url
	userinfoResponse, userinfoError := client.Get(strings.Replace(conf.Endpoint.AuthURL,"/authorization","/userinfo",1))
	if userinfoError != nil {
		return nil, errors.New("Failed to obtain userinfo:"+userinfoError.Error())
	}

	defer userinfoResponse.Body.Close()

	// Decoding profile info and putting it in a map, to make it more readable
	var profile map[string]interface{}
	if userinfoError = json.NewDecoder(userinfoResponse.Body).Decode(&profile); userinfoError != nil {
		return nil, userinfoError
	}

	return profile, nil

}

// Home handler for /home
func home(w http.ResponseWriter, r *http.Request) {

	log.Println("Executing /home")

	// Parssing home.html template
	tmpl,_ := template.ParseFiles("./static/home.html")
	data := &Home{}

	// Adding title to page
	data.Title = "Welcome to AppID"

	// Getting cookie named SESSION_TOKEN
	cookie, err := r.Cookie(SESSION_TOKEN)

	if err != nil{

		// If no cookie found, that's ok, that means no user is logged in
		log.Println("No session cookie found:"+err.Error())

	}else {

		log.Println("Session cookie found.")

		// A cookie was found, this means a user is logged in
		// Let's get the auth token value

		authToken := oauth2.Token{
			AccessToken: cookie.Value,
		}

		// Getting the user profile for the given auth token
		profile, profileError := GetUserProfile(r, authToken)

		if profileError != nil {
			log.Print("Error getting profile.")
		}

		// Setting values in page template, this is what we are going to show for the logged in user
		data.User.Token = fmt.Sprintln(authToken.AccessToken)
		data.User.Profile = fmt.Sprintln(profile)

		log.Println("User already logged in:" + fmt.Sprintln(profile))

	}

	tmpl.ExecuteTemplate(w, "home", data)

}

// Login handler for /login
func login(w http.ResponseWriter, r *http.Request) {

	log.Println("Executing /login")

	// Code request to Auth URL
	http.Redirect(w, r, conf.AuthCodeURL(STATE), http.StatusFound)

}

// Callback handler for /auth/callback
func callback(w http.ResponseWriter, r *http.Request) {

	log.Println("Executing /callback")

	// Getting auth token from request
	authToken, error := GetOauthToken(r)

	if error != nil{

		log.Println("Error getting auth token.")

	}else {

		log.Println("Setting session cookie.")

		// Setting cookie with the value of this auth token

		http.SetCookie(w, &http.Cookie{
			Name:    "session_token",
			Value:   authToken.AccessToken,
			Path:    "/",
			Expires: time.Now().Add(1000 * time.Second),
		})

	}

	// Redirecting to /home, in order to show the logged in user values
	http.Redirect(w, r, "/home",http.StatusSeeOther)

}

// Logout handler for /logout
func logout(w http.ResponseWriter, r *http.Request) {

	log.Println("Executing /logout")

	// Getting session cookie
	cookie, err := r.Cookie(SESSION_TOKEN)

	if err != nil{

		log.Println("No session cookie found:"+err.Error())

	}else{

		log.Println("Session cookie found, invalidating it.")

		// If cookie was found, let's invalidate it
		cookie.MaxAge = -1

	}

	// Setting the invalidated cookie
	http.SetCookie(w, cookie)

	// Redirecting to home for login screen
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func main() {

	log.Println("Starting appid execution.")
	// Loading App Id configuration file
	app_id_configuration,  app_id_configuration_error := loadConfigurationFile()

	if app_id_configuration_error != nil{

		log.Println("Could not load configuration file.")

	}

	// Building global conf object, using App Id configuration
	conf = buildConfigurationObject(app_id_configuration)

	// Serving static files
	fs := http.FileServer(http.Dir("static"))

	// Creating handlers: /static /home /login /auth/callback /logout

	http.Handle("/static/", http.StripPrefix("/static/",fs))

	http.HandleFunc("/home", home)

	http.HandleFunc("/login", login)

	http.HandleFunc("/auth/callback", callback)

	http.HandleFunc("/logout", logout)

	// Using port 3000
	port := ":3000"

	log.Println("Listening on port ", port)

	http.ListenAndServe(port, nil)

}

