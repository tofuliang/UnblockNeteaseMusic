package opensubsonic

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cnsilvan/UnblockNeteaseMusic/common"
	"github.com/cnsilvan/UnblockNeteaseMusic/config"
	"github.com/cnsilvan/UnblockNeteaseMusic/provider/base"
	"github.com/supersonic-app/go-subsonic/subsonic"
)

var clients []subsonic.Client
var lastUsedClient *subsonic.Client
var lastUsedTime time.Time
var handle *time.Timer

type OpenSubsonic struct {
}

type account struct {
	Username string `json:"username"`
	Password string `json:"password"`
	BaseUrl  string `json:"baseUrl"`
}

func Init() {
	accounts := parseAccounts(*config.OpenSubsonicConfig)
	for _, account := range accounts {
		client := subsonic.Client{
			UserAgent:    `Submariner/3.2.1`,
			Client:       &http.Client{Timeout: 60 * time.Second},
			BaseUrl:      account.BaseUrl,
			User:         account.Username,
			PasswordAuth: false,
			ClientName:   `submariner`,
		}
		err := client.Authenticate(account.Password)
		if err == nil {
			clients = append(clients, client)
		}
	}
}

func (o OpenSubsonic) SearchSong(song common.SearchSong) (songs []*common.Song) {
	client, err := getClient()
	if err != nil {
		return songs
	}

	searchResult, err := client.Search2(song.Keyword, map[string]string{"artistCount": "0", "albumCount": "0"})
	if err != nil {
		return songs
	}
	listLength := len(searchResult.Song)
	maxIndex := listLength/2 + 1
	if maxIndex > 10 {
		maxIndex = 10
	}
	var tempSongs []*common.Song
	sort.Slice(searchResult.Song, func(i, j int) bool {
		return searchResult.Song[i].Year < searchResult.Song[j].Year
	})
	for index, result := range searchResult.Song {
		if index >= maxIndex {
			break
		}
		url, err := client.GetStreamURL(result.ID, map[string]string{"maxBitRate": "0", "format": "raw"})
		if err != nil {
			continue
		}
		songResult := &common.Song{
			Id:        string(common.OpenSubsonicTag) + result.ID,
			Size:      result.Size,
			Br:        result.BitRate * 1000,
			Url:       url.String(),
			Name:      result.Title,
			Artist:    result.Artist,
			AlbumName: result.Album,
			Duration:  result.Duration,
			Source:    `OpenSubsonic`,
		}
		songResult.PlatformUniqueKey = map[string]interface{}{}
		songResult.PlatformUniqueKey["UnKeyWord"] = song.Keyword
		songResult.PlatformUniqueKey["MusicId"] = result.ID
		songResult.PlatformUniqueKey["songType"] = result.Suffix
		ok := false
		songResult.MatchScore, ok = base.CalScore(song, result.Title, result.Artist, result.Album, index, maxIndex)
		if !ok {
			continue
		}
		tempSongs = append(tempSongs, songResult)
	}

	sort.Slice(tempSongs, func(i, j int) bool {
		return tempSongs[i].MatchScore > tempSongs[j].MatchScore
	})

	songs = tempSongs
	return
}

func (o OpenSubsonic) GetSongUrl(searchSong common.SearchMusic, song *common.Song) *common.Song {
	if handle != nil {
		handle.Stop()
	}
	client, err := getClient()
	if err != nil {
		return song
	}
	if song.Duration > 0 {
		var duration int
		if song.Duration > 480 {
			duration = 240
		} else {
			duration = song.Duration / 2
		}
		client.Scrobble(song.PlatformUniqueKey["MusicId"].(string), map[string]string{
			"time":       strconv.FormatInt(time.Now().UnixMilli(), 10),
			"submission": "false"})

		handle = time.AfterFunc(time.Duration(duration)*time.Second, func() {
			client.Scrobble(song.PlatformUniqueKey["MusicId"].(string), map[string]string{
				"time":       strconv.FormatInt(time.Now().UnixMilli(), 10),
				"submission": "true"})

			handle.Stop()
		})
	}

	return song
}

func (o OpenSubsonic) ParseSong(searchSong common.SearchSong) *common.Song {
	song := &common.Song{}
	songs := o.SearchSong(searchSong)
	if len(songs) > 0 {
		song = o.GetSongUrl(common.SearchMusic{Quality: searchSong.Quality}, songs[0])
	}
	return song
}

func getClient() (subsonic.Client, error) {
	var mu sync.Mutex

	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	if lastUsedClient != nil && now.Sub(lastUsedTime) < 10*time.Minute {
		return *lastUsedClient, nil
	}

	if len(clients) == 0 {
		return subsonic.Client{}, fmt.Errorf("no clients available")
	}

	// Select a random client
	randomIndex := rand.Intn(len(clients))
	selectedClient := &clients[randomIndex]

	lastUsedClient = selectedClient
	lastUsedTime = now

	return *selectedClient, nil
}

func parseAccounts(file string) []account {
	fl, err := os.Open(file)
	if err != nil {
		fmt.Println(file, err)
		return nil
	}
	defer fl.Close()
	decoder := json.NewDecoder(fl)
	var accounts []account
	// 解析 JSON 文件并将结果存储在 accounts 切片中
	err = decoder.Decode(&accounts)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return nil
	}

	// 返回解析后的账户信息
	return accounts
}
