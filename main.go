package main

import (
	"log"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/slack-go/slack"
)

const (
	url    = "https://slack.com/api/files.upload"
	region = "ap-northeast-1"
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

	queueURL = os.Getenv("PETCAM_QUEUE_URL")
	if channelID == "" {
		log.Fatal("$PETCAM_QUEUE_URL is empty")
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{Config: aws.Config{Region: aws.String(region)}, Profile: "petcam"}))
	svc = sqs.New(sess)

	log.Println("start polling")
	for {
		// log.Println("start receive messages")
		msgs, err := receiveMessages()
		if err != nil {
			log.Fatal(err)
		}
		if len(msgs) == 0 {
			// log.Println("no queues")
			continue
		}

		log.Println("take video")
		err = takeVideo()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("send slack")
		err = sendSlack()
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

func sendSlack() error {
	file, err := os.Open("test.mp4")
	if err != nil {
		return err
	}

	api := slack.New(token)

	params := slack.FileUploadParameters{
		Reader:   file,
		Filename: "movie.mp4",
		Channels: []string{channelID},
	}

	_, err = api.UploadFile(params)
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
