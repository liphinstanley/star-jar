package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// inputError is a mistake in what the parent typed; it is shown back on the page
type inputError string

func (e inputError) Error() string { return string(e) }

type taskInput struct {
	Title string
	Icon  string
	Stars int
}

type rewardInput struct {
	Title string
	Icon  string
	Cost  int
	Size  string
}

func readTask(r *http.Request) (taskInput, error) {
	var in taskInput
	var err error
	if in.Icon, err = readText(r, "icon", 8); err != nil {
		return in, err
	}
	if in.Title, err = readText(r, "title", 40); err != nil {
		return in, err
	}
	if in.Stars, err = readNumber(r, "stars", 1, 20); err != nil {
		return in, err
	}
	return in, nil
}

func readReward(r *http.Request) (rewardInput, error) {
	var in rewardInput
	var err error
	if in.Icon, err = readText(r, "icon", 8); err != nil {
		return in, err
	}
	if in.Title, err = readText(r, "title", 40); err != nil {
		return in, err
	}
	if in.Cost, err = readNumber(r, "cost", 1, 1000); err != nil {
		return in, err
	}
	in.Size = r.FormValue("size")
	if in.Size != "small" && in.Size != "big" {
		return in, inputError("size must be small or big")
	}
	return in, nil
}

// readText returns a trimmed form field with 1 to max characters
func readText(r *http.Request, name string, max int) (string, error) {
	v := strings.TrimSpace(r.FormValue(name))
	if v == "" || utf8.RuneCountInString(v) > max {
		return "", inputError(name + " must be 1 to " + strconv.Itoa(max) + " characters")
	}
	return v, nil
}

// readNumber returns a whole-number form field between min and max
func readNumber(r *http.Request, name string, min, max int) (int, error) {
	n, err := strconv.Atoi(r.FormValue(name))
	if err != nil || n < min || n > max {
		return 0, inputError(name + " must be a number from " + strconv.Itoa(min) + " to " + strconv.Itoa(max))
	}
	return n, nil
}
