package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/google/go-github/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultConfigName = "osprey"
	defaultConfigType = "yml"
	defaultConfigPath = "/usr/local/etc/"
	defaultRootKey = "services"
	githubAuthEnvKey = "GITHUB_AUTH_TOKEN"
	defaultErrorKeyword = "error"
)

// scanner defines log file scanner.
type scanner struct {
	// client is a github API service client. A client is shared.
	client *github.Client

	// service contains service log info.
	service *service

	// iguFilePath is the .igu file path for this service.
	iguFilePath string

	// anchor is the last visited line number from previous scanning task.
	anchor int
}

// service holds the information about service, including log file location and target repository.
type service struct {
	// name is the service name.
	name string

	// logFileLoc is the log file location.
	logFileLoc string

	// repoOwner is the target repository owner.
	repoOwner string

	// repoName is the target repository name.
	repoName string
}

// execute executes the scanning job for the given service.
func (s *scanner) Execute(ctx context.Context) error {
	issReqs, err := s.scan()
	if err != nil {
		return err
	}

	n := len(issReqs)
	if n > 0 {
		log.Printf("%d new errors detected\n", n)

		for _, issReq := range issReqs {
			_, _, err = s.client.Issues.Create(ctx, s.service.repoOwner, s.service.repoName, issReq)
			if err != nil {
				log.Printf("%s\n", err.Error())
			}
		}
	}

	return nil
}

// scan scans the log file from the last visited line to the end.
func (s *scanner) scan() ([]*github.IssueRequest, error) {
	// Read latest author info.
	err := s.getAnchor()
	if err != nil {
		return nil, err
	}

	newAnchor, issues, err := s.scanFile()
	if err != nil {
		return nil, err
	}
	if newAnchor > s.anchor {
		err := s.setAnchor(newAnchor)
		if err != nil {
			return nil, err
		}
	}

	return issues, nil
}

// scanFile scans log file based on last set anchor.
// If new error logs are found, wrap them into github's issue request.
func (s *scanner) scanFile() (newAnchor int, issues []*github.IssueRequest, err error) {
	dat, err := ioutil.ReadFile(s.service.logFileLoc)
	if err != nil {
		return s.anchor, nil, err
	}

	// Read from the current anchor.
	var (
		forward = 0
	)

	logs := strings.Split(string(dat), "\n")
	unread := logs[s.anchor: ]
	unreadStr := strings.Join(unread, "\n")
	fScanner := bufio.NewScanner(strings.NewReader(unreadStr))

	for fScanner.Scan() {
		if strings.Contains(fScanner.Text(), defaultErrorKeyword) {
			title := title(s.service.name)
			body := fScanner.Text()
			issues = append(issues, &github.IssueRequest{
				Title: &title,
				Body: &body,
			})
		}
		forward += 1
	}

	newAnchor = s.anchor + forward

	if err := fScanner.Err(); err != nil {
		return newAnchor, issues, err
	}

	return newAnchor, issues, nil
}

// reloadLastLine reloads last line for each service since we may update the data.
func (s *scanner) getAnchor() error {
	// Check out if a file exists, if not, create one.
	var _, err = os.Stat(s.iguFilePath)

	var f *os.File
	if os.IsNotExist(err) {
		f, err = os.Create(s.iguFilePath)
		if err != nil {
			return err
		}
		defer f.Close()

		w := bufio.NewWriter(f)
		_, err := w.WriteString("last:0\n")
		if err != nil {
			return err
		}
	}

	// Read out the anchor save earlier.
	f, err = os.Open(s.iguFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if err != nil {
		return err
	}

	// Create a file scanner for reading
	fScanner := bufio.NewScanner(r)

	var anchorLine string
	for fScanner.Scan() {
		// We assume only one line. So we break after read one line.
		anchorLine = fScanner.Text()
		break
	}

	if err := fScanner.Err(); err != nil {
		return err
	}

	// If the line is empty, we assume log file for a given service has never been read by Iguana before.
	if anchorLine == "" {
		return nil
	}

	anc, err := extract(anchorLine)
	if err != nil {
		return err
	}

	s.anchor = anc

	return nil
}

// setAnchor updates anchor info in the file
func (s *scanner) setAnchor(newAnchor int) error {
	var lock = sync.RWMutex{}
	lock.Lock()
	defer lock.Unlock()

	f, err := os.OpenFile(s.iguFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("last:%d\n", newAnchor))
	if err != nil {
		return err
	}

	return nil
}


// readConfig reads osprey config file.
func readConfig() error {
	viper.SetConfigName(defaultConfigName)
	viper.SetConfigType(defaultConfigType)
	viper.AddConfigPath(defaultConfigPath)

	viper.WatchConfig()

	// Read the config file.
	return viper.ReadInConfig()
}

// createScanners creates scanner based on config file.
func createScanners(client *github.Client) (scanners []*scanner, err error) {
	// Read iguFilePath.
	iguFilePath := viper.GetString("igu_file_path")

	// Read service configurations
	services := viper.GetStringMap(defaultRootKey)
	for name, cfg := range services {
		loc := cfg.(map[string]interface{})["location"].(string)
		repoOwner := cfg.(map[string]interface{})["repo_owner"].(string)
		repoName := cfg.(map[string]interface{})["repo_name"].(string)
		scanners = append(scanners, &scanner{
			client:      client,
			iguFilePath: fmt.Sprintf("%s/%s.igu", iguFilePath, name),
			service: &service{
				name:       name,
				logFileLoc: loc,
				repoOwner:  repoOwner,
				repoName:   repoName,
			},
		})
	}

	return scanners, nil
}

// connect() gets a connected github API service client
func connect(ctx context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: strings.TrimSpace(os.Getenv(githubAuthEnvKey))},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

// extract extracts anchor info.
func extract(line string) (int, error) {
	tks := strings.Split(line, ":")
	if len(tks) == 0 {
		return 0, nil
	}

	anchor, err := strconv.Atoi(tks[1])
	if err != nil {
		return 0, err
	}

	return anchor, nil
}

// title returns issue title given service name.
func title(serviceName string) string {
	return fmt.Sprintf("%s-bug-%s", serviceName, time.Now().Format("2006-01-02 15:04:05"))
}

func main() {
	ctx := context.Background()

	// Read config file.
	err := readConfig()
	if err != nil {
		log.Fatalf("Unable to start Iguana, %s", err.Error())
	}
	// Read osprey configurations first.
	interval := viper.GetInt("interval")
	maxWorkers := viper.GetInt("max_workers")

	// Obtain github API client.
	c := connect(ctx)
	if c == nil {
		log.Fatal("Unable to start Iguana, fail to obtain github API service client.")
	}

	// Create scanners based on the services defined in the config file.
	scanners, err := createScanners(c)
	if err != nil {
		log.Fatalf("Unable to start Iguana, %s", err.Error())
	}

	if scanners == nil || len(scanners) == 0 {
		log.Fatalf("Unable to start Iguana, no jobs are found.")
	}

	// Setup a worker pool
	workerN := len(scanners)
	if workerN >  maxWorkers {
		workerN = maxWorkers
	}

	log.Printf("%d scanners are created.\n", len(scanners))
	queue := make(chan scanner, workerN)

	t := time.NewTicker(time.Duration(interval) * time.Second)
	log.Println("osprey is ready")
	for range t.C {
		// Execute jobs.
		for i := 1; i <= workerN; i++ {
			// start a worker
			go func() {
				for {
					scanner := <-queue

					// execute the job
					if err := scanner.Execute(ctx); err != nil {
						log.Printf("%s.\n", err.Error())
					}
				}
			}()
		}

		// Push scanners to queue
		for _, scanner := range scanners {
			queue <- *scanner
		}
	}
}



