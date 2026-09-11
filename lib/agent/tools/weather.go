package tools

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/openai/openai-go/v3/shared"
)

var Weather = register(Tool{
	Name: "get_weather",
	Description: "Gets the current weather for a named location, e.g. \"Oslo\" or " +
		"\"Paris, France\".",
	Parameters: shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "The place to look up, e.g. \"Bergen, Norway\".",
			},
		},
		"required": []string{"location"},
	},
	Call: func(arguments string) (string, error) {
		var args struct {
			Location string `json:"location"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("parse arguments: %w", err)
		}

		lat, lon, name, err := geocode(args.Location)
		if err != nil {
			return "", fmt.Errorf("look up location %q: %w", args.Location, err)
		}
		weather, err := currentWeather(lat, lon)
		if err != nil {
			return "", fmt.Errorf("fetch weather for %q: %w", name, err)
		}
		return fmt.Sprintf(
			"%s: %.1f°C, wind %.1f km/h",
			name,
			weather.temperature,
			weather.windSpeed,
		), nil
	},
})

// geocode resolves a free-text location to coordinates via Open-Meteo's
// (free, keyless) geocoding API, since the forecast API itself only takes
// latitude/longitude.
func geocode(location string) (lat, lon float64, name string, err error) {
	u := "https://geocoding-api.open-meteo.com/v1/search?count=1&name=" + url.QueryEscape(location)
	var result struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Name      string  `json:"name"`
			Country   string  `json:"country"`
		} `json:"results"`
	}
	if err := getJSON(u, &result); err != nil {
		return 0, 0, "", err
	}
	if len(result.Results) == 0 {
		return 0, 0, "", fmt.Errorf("no matching location")
	}
	r := result.Results[0]
	return r.Latitude, r.Longitude, fmt.Sprintf("%s, %s", r.Name, r.Country), nil
}

type weatherReading struct {
	temperature float64
	windSpeed   float64
}

func currentWeather(lat, lon float64) (weatherReading, error) {
	u := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current_weather=true",
		lat, lon,
	)
	var result struct {
		CurrentWeather struct {
			Temperature float64 `json:"temperature"`
			WindSpeed   float64 `json:"windspeed"`
		} `json:"current_weather"`
	}
	if err := getJSON(u, &result); err != nil {
		return weatherReading{}, err
	}
	return weatherReading{
		temperature: result.CurrentWeather.Temperature,
		windSpeed:   result.CurrentWeather.WindSpeed,
	}, nil
}
