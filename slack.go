package main

import (
	"os"

	"github.com/slack-go/slack"
)

type Slack struct {
	api       *slack.Client
	channelID string
}

func (s *Slack) sendMovie() error {
	file, err := os.Open("test.mp4")
	if err != nil {
		return err
	}

	params := slack.FileUploadParameters{
		Reader:   file,
		Filename: "movie.mp4",
		Channels: []string{s.channelID},
	}

	_, err = s.api.UploadFile(params)
	if err != nil {
		return err
	}

	return nil
}

func (s *Slack) sendMessage(msg string) error {
	slackMessage := slack.MsgOptionText(
		msg,
		true,
	)

	_, _, err := s.api.PostMessage(s.channelID, slackMessage)
	if err != nil {
		return err
	}

	return nil
}

func CreateSlack(token, channelID string) *Slack {
	return &Slack{
		slack.New(token),
		channelID,
	}
}
