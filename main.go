package main

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

const (
	url      = "https://slack.com/api/files.upload"
	maxRetry = 3
)

var (
	svc       *sqs.SQS
	token     string
	channelID string
	queueURL  string
)

func main() {
	token = os.Getenv("PETCAM_SLACK_TOKEN")
	if token == "" {
		log.Fatal("$PETCAM_SLACK_TOKEN is empty")
	}

	channelID = os.Getenv("PETCAM_SLACK_CHANNEL_ID")
	if channelID == "" {
		log.Fatal("$PETCAM_SLACK_CHANNEL_ID is empty")
	}

	slack := CreateSlack(token, channelID)

	queueURL = os.Getenv("PETCAM_QUEUE_URL")
	if channelID == "" {
		log.Fatal("$PETCAM_QUEUE_URL is empty")
	}

	sess := session.Must(
		session.NewSession(&aws.Config{
			Retryer: CustomRetryer{
				DefaultRetryer: client.DefaultRetryer{
					NumMaxRetries: client.DefaultRetryerMaxNumRetries,
					MinRetryDelay: client.DefaultRetryerMinRetryDelay,
					MaxRetryDelay: client.DefaultRetryerMaxRetryDelay,
				}},
			Credentials: credentials.NewCredentials(&credentials.SharedCredentialsProvider{
				Filename: defaults.SharedCredentialsFilename(),
				Profile:  "petcam",
			}),
			Region: aws.String(endpoints.ApNortheast1RegionID),
		}),
	)
	svc = sqs.New(sess)

	errCount := 0

	log.Println("start polling")
	for {
		// log.Println("start receive messages")
		msgs, err := receiveMessages()
		if err != nil {
			log.Println("failed to get queue")
			err := slack.sendMessage("queueの取得に失敗しました。 err: " + err.Error())
			if err != nil {
				log.Println("failed to send slack a message")
			}

			errCount = errCount + 1
			if errCount >= maxRetry {
				log.Println("reach to max retry count")
				err := slack.sendMessage("最大リトライ数に達しました。機能を停止します")
				if err != nil {
					log.Println("failed to send slack a message")
				}

				log.Fatal(err)
			}

			err = slack.sendMessage("機能を一時停止します。")
			if err != nil {
				log.Fatal(err)
			}
			log.Println("sleep")
			time.Sleep(1 * time.Minute)

			log.Println("restart")
			err = slack.sendMessage("再開します")
			if err != nil {
				log.Fatal(err)
			}
			continue
		}
		if len(msgs) == 0 {
			// log.Println("no queues")
			continue
		}

		log.Println("send start message to slack")
		err = slack.sendMessage("撮影開始")
		if err != nil {
			log.Fatal(err)
		}

		log.Println("take video")
		err = takeVideo()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("send video to slack")
		err = slack.sendMovie()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("delete all queues")
		err = deleteAllQueue(msgs)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("delete video")
		err = deleteVideo()
		if err != nil {
			log.Fatal(err)
		}
	}
}

func takeVideo() error {
	err := exec.Command("raspivid", "-vf", "-hf", "-o", "test.h264", "-w", "640", "-h", "480", "-t", "10000").Run()
	if err != nil {
		return err
	}

	err = exec.Command("MP4Box", "-fps", "30", "-add", "test.h264", "test.mp4").Run()
	if err != nil {
		return err
	}

	return nil
}

func deleteVideo() error {
	err := exec.Command("rm", "test.h264", "test.mp4").Run()
	if err != nil {
		return err
	}

	return nil
}

func receiveMessages() ([]*sqs.Message, error) {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: aws.Int64(10),
		WaitTimeSeconds:     aws.Int64(20),
	}
	resp, err := svc.ReceiveMessage(input)
	if err != nil {
		return []*sqs.Message{}, err
	}

	return resp.Messages, nil
}

func deleteAllQueue(msgs []*sqs.Message) error {
	for _, msg := range msgs {
		input := &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(queueURL),
			ReceiptHandle: aws.String(*msg.ReceiptHandle),
		}

		_, err := svc.DeleteMessage(input)
		if err != nil {
			return err
		}
	}

	return nil
}

// Custom Retry
// cf. https://github.com/aws/aws-sdk-go/blob/main/example/aws/request/customRetryer/custom_retryer.go
type CustomRetryer struct {
	client.DefaultRetryer
}

type temporary interface {
	Temporary() bool
}

func (r CustomRetryer) ShouldRetry(req *request.Request) bool {
	if origErr := req.Error; origErr != nil {
		log.Println(origErr.Error())
		switch origErr.(type) {
		case temporary:
			if strings.Contains(origErr.Error(), "read: connection reset") {
				return true
			}
		}
	}
	return r.DefaultRetryer.ShouldRetry(req)
}
