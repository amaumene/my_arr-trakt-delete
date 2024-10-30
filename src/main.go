package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jacklaaa89/trakt"
	"github.com/jacklaaa89/trakt/authorization"
	"github.com/jacklaaa89/trakt/sync"
	"golift.io/starr"
	"golift.io/starr/sonarr"
)

func main() {
	trakt.Key = os.Getenv("TRAKT_API_KEY")
	clientSecret := os.Getenv("TRAKT_CLIENT_SECRET")

	if trakt.Key == "" || clientSecret == "" {
		log.Fatalf("TRAKT_API_KEY and TRAKT_CLIENT_SECRET must be set in environment variables")
	}

	sonarrApiKey := os.Getenv("SONARR_API_KEY")
	sonarrUrl := os.Getenv("SONARR_URL")
	if sonarrApiKey == "" || sonarrUrl == "" {
		log.Fatalf("SONARR_API_KEY and SONARR_URL must be set in environment variables")
	}
	tokenPath := os.Getenv("TOKEN_PATH")
	if tokenPath == "" {
		log.Printf("TOKEN_PATH not set, using current directory")
		tokenPath = "."
	}
	tokenFile := tokenPath + "/token.json"

	token, err := getToken(clientSecret, tokenFile)
	if err != nil {
		log.Fatalf("Error getting token: %v", err)
	}

	params := trakt.ListParams{OAuth: token.AccessToken}

	historyParams := &trakt.ListHistoryParams{
		ListParams: params,
		Type:       "shows",
		EndAt:      time.Now(),
		StartAt:    time.Now().AddDate(0, 0, -7),
	}
	iterator := sync.History(historyParams)

	c := starr.New(sonarrApiKey, sonarrUrl, 0)
	s := sonarr.New(c)

	processHistory(iterator, s)
}

func getToken(clientSecret string, tokenFile string) (*trakt.Token, error) {
	if _, err := os.Stat(tokenFile); err == nil {
		return loadTokenFromFile(tokenFile)
	}
	return generateNewToken(clientSecret, tokenFile)
}

func loadTokenFromFile(tokenFile string) (*trakt.Token, error) {
	file, err := os.Open(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("error opening token file: %v", err)
	}
	defer file.Close()

	var token trakt.Token
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&token); err != nil {
		return nil, fmt.Errorf("error decoding token from JSON: %v", err)
	}
	return &token, nil
}

func generateNewToken(clientSecret string, tokenFile string) (*trakt.Token, error) {
	deviceCode, err := authorization.NewCode(nil)
	if err != nil {
		return nil, fmt.Errorf("error generating device code: %v", err)
	}

	fmt.Printf("Please go to %s and enter the code: %s\n", deviceCode.VerificationURL, deviceCode.UserCode)

	pollParams := &trakt.PollCodeParams{
		Code:         deviceCode.Code,
		Interval:     deviceCode.Interval,
		ExpiresIn:    deviceCode.ExpiresIn,
		ClientSecret: clientSecret,
	}
	token, err := authorization.Poll(pollParams)
	if err != nil {
		return nil, fmt.Errorf("error polling for token: %v", err)
	}

	if err := saveTokenToFile(token, tokenFile); err != nil {
		return nil, err
	}
	return token, nil
}

func saveTokenToFile(token *trakt.Token, tokenFile string) error {
	file, err := os.Create(tokenFile)
	if err != nil {
		return fmt.Errorf("error creating token file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(token); err != nil {
		return fmt.Errorf("error encoding token to JSON: %v", err)
	}
	return nil
}

func processHistory(iterator *trakt.HistoryIterator, s *sonarr.Sonarr) {
	for iterator.Next() {
		item, err := iterator.History()
		if err != nil {
			log.Fatalf("Error scanning item: %v", err)
		}

		fmt.Printf("Trakt Watched: %s - S%02dE%02d - %s on %s\n", item.Show.Title, item.Episode.Season, item.Episode.Number, item.Episode.Title, item.WatchedAt)
		processSonarrEpisodes(s, item)
	}

	if err := iterator.Err(); err != nil {
		log.Fatalf("Error iterating history: %v", err)
	}
}

func processSonarrEpisodes(s *sonarr.Sonarr, item *trakt.History) {
	s_serie, err := s.GetSeries(int64(item.Show.TVDB))
	if err != nil {
		log.Fatalf("Error getting series: %v", err)
	}
	if s_serie[0].ID != 0 {
		s_episodes, err := s.GetSeriesEpisodes(s_serie[0].ID)
		if err != nil {
			log.Fatalf("Error getting episodes: %v", err)
		}
		for _, episode := range s_episodes {
			if episode.SeasonNumber == item.Episode.Season && episode.EpisodeNumber == item.Episode.Number {
				if episode.HasFile {
					fmt.Printf("Sonarr Episode: %s - %s has file\n", item.Show.Title, episode.Title)
					err = s.DeleteEpisodeFile(episode.ID)
					if err != nil {
						log.Fatalf("Error deleting file: %v", err)
					}
					fmt.Printf("Sonarr Episode: %s - %s file deleted\n", item.Show.Title, episode.Title)
				}
			}
		}
	}
}
