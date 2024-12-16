package main

import (
	"fmt"
	"time"

	"github.com/manifoldco/promptui"
)

func InteractiveSelect[T any](items []T, formatter func(item T) string) (int, error) {
	strItem := make([]string, len(items))
	for i := range items {
		strItem[i] = formatter(items[i])
	}
	listSize := len(strItem)
	if listSize > 10 {
		listSize = 10
	}
	prompt := promptui.Select{
		Label: "Select Node",
		Items: strItem,
		Size:  listSize,
	}
	i, _, err := prompt.Run()
	if err != nil {
		return -1, err
	}
	return i, nil
}

func FormatDuration(d time.Duration) string {
	if d > 24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	} else if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
