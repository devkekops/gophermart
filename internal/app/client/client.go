package client

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

const baseQuery = "/api/orders/"

type AccrualResponse struct {
	StatusCode int
	Order      string  `json:"order"`
	Status     string  `json:"status"`
	Accrual    float64 `json:"accrual"`
}

type Client interface {
	GetAccrualInfo(number string) (AccrualResponse, error)
}

type cli struct {
	host       string
	httpClient *http.Client
}

func NewCli(host string, timeout int) Client {
	client := &http.Client{
		Timeout: time.Duration(timeout * int(time.Second)),
	}
	return &cli{
		host:       host,
		httpClient: client,
	}
}

func (c *cli) GetAccrualInfo(number string) (AccrualResponse, error) {
	var accrualResp AccrualResponse
	baseURL := c.host + baseQuery + number
	res, err := c.httpClient.Get(baseURL)
	if err != nil {
		return accrualResp, err
	}
	defer res.Body.Close()

	accrualResp.StatusCode = res.StatusCode
	if res.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return accrualResp, err
		}
		if err = json.Unmarshal(body, &accrualResp); err != nil {
			return accrualResp, err
		}
	}
	return accrualResp, nil
}
