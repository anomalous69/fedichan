package config

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

var Port = ":" + GetConfigValue("instanceport", "3000")
var TP = GetConfigValue("instancetp", "")
var Domain = TP + "" + GetConfigValue("instance", "")
var InstanceName = GetConfigValue("instancename", "")
var InstanceSummary = GetConfigValue("instancesummary", "")
var SiteEmail = GetConfigValue("emailaddress", "")                                       //contact@fchan.xyz
var SiteEmailUser = GetConfigValue("emailuser", SiteEmail)                               //contact@fchan.xyz
var SiteEmailFrom = GetConfigValue("emailfrom", fmt.Sprintf("FChannel <%s>", SiteEmail)) // FChannel <contact@fchan.xyz>
var SiteEmailPassword = GetConfigValue("emailpass", "")
var SiteEmailSMTP = GetConfigValue("emailsmtp", fmt.Sprintf("%s:%s", SiteEmailServer, SiteEmailPort))
var TorProxy = GetConfigValue("torproxy", "") //127.0.0.1:9050
var Salt = GetConfigValue("instancesalt", "")
var DBHost = GetConfigValue("dbhost", "localhost")
var DBPort, _ = strconv.Atoi(GetConfigValue("dbport", "5432"))
var DBUser = GetConfigValue("dbuser", "postgres")
var DBPassword = GetConfigValue("dbpass", "password")
var DBName = GetConfigValue("dbname", "server")
var CookieKey = GetConfigValue("cookiekey", "")
var ActivityStreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""
var PostCountPerPage = 10
var SupportedFiles = []string{"image/gif", "image/jpeg", "image/png", "image/webp", "image/apng", "video/mp4", "video/ogg", "video/webm", "audio/mpeg", "audio/ogg", "audio/wav", "audio/wave", "audio/x-wav"}
var MediaHashs = make(map[string]string)
var Key = GetConfigValue("modkey", "")
var Debug = GetConfigValue("debug", "")
var Themes []string
var DB *sql.DB

// Deprecated
var (
	SiteEmailServer   = GetConfigValue("emailserver", "")
	SiteEmailPort     = GetConfigValue("emailport", "smtp")
	SiteEmailNotifyTo = GetConfigValue("emailnotify", "")
)

var deprecated = map[string]string{
	"emailserver": "Specify address:port in emailsmtp instead.",
	"emailport":   "Specify address:port in emailsmtp instead.",
	"emailnotify": "This option no longer has any effect. All users who have a registered email address are notified.",
}

// TODO Change this to some other config format like YAML
// to save into a struct and only read once
func GetConfigValue(value string, ifnone string) string {
	file, err := os.Open("fchan.cfg")

	if err != nil {
		log.Println(err)
		return ifnone
	}

	defer file.Close()

	lines := bufio.NewScanner(file)

	for lines.Scan() {
		line := strings.SplitN(lines.Text(), ":", 2)

		if line[0] == value {
			if msg, ok := deprecated[value]; ok {
				log.Printf("Config key \"%s\" is deprecated: %s", value, msg)
			}

			return line[1]
		}
	}

	return ifnone
}

func IsEmailSetup() bool {
	return SiteEmail != ""
}
