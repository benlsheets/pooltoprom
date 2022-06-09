package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	addr    = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	poolURL = flag.String("pool-url", "", "The URL of the pool API endpoint.")
	wallet  = flag.String("wallet", "", "The wallet ID being monitored.")

	pool_balance_paid = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_paid",
		Help: "Balance paid from pool (COINe-9)",
	},
		[]string{"pool"},
	)

	pool_balance_unpaid = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_unpaid",
		Help: "Unpaid balance on pool (COINe-9)",
	},
		[]string{"pool"},
	)

	pool_balance_unconfirmed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_unconfirmed",
		Help: "Unconfirmed balance on pool (COINe-9)",
	},
		[]string{"pool"},
	)

	pool_rewards = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pool_rewards",
		Help: "Total pool rewards (COINe-9)",
	},
		[]string{"pool"},
	)

	pool_hashrate_current = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_current",
		Help: "Current Worker Hashrate (H/s)",
	},
		[]string{"pool", "worker"},
	)

	pool_hashrate_average = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_average",
		Help: "Average Worker Hashrate (H/s)",
	},
		[]string{"pool", "worker"},
	)

	pool_hashrate_reported = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_reported",
		Help: "Reported Worker Hashrate (H/s)",
	},
		[]string{"pool", "worker"},
	)

	pool_shares_valid = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pool_shares_valid",
		Help: "Valid Worker Shares",
	},
		[]string{"pool", "worker"},
	)

	pool_shares_invalid = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pool_shares_invalid",
		Help: "Invalid Worker Shares",
	},
		[]string{"pool", "worker"},
	)

	pool_shares_stale = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pool_shares_stale",
		Help: "Stale Worker Shares",
	},
		[]string{"pool", "worker"},
	)

	pool_info Pool

	worker_info = make(map[string]Worker)
)

func recordMetrics() {
	go func() {
		requestURL := fmt.Sprintf("https://%s/api/accounts/%s", *poolURL, *wallet)
		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("accept", "application/json")

		for {
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				ParseJSONresponse(res)
			} else {
				log.Println("WARN: Error sending API request:", err, "Response:", res.StatusCode, res.Status)
			}

			time.Sleep(1 * time.Minute)
		}
	}()
}

func ParseJSONresponse(response *http.Response) {
	resBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Println("WARN: Error reading API response.", err)
	}

	var json_data interface{}
	err = json.Unmarshal(resBody, &json_data)
	if err != nil {
		log.Println("WARN: Error parsing API response.", err)
	}

	json_map, ok := json_data.(map[string]interface{})
	if !ok {
		log.Println("WARN: Invalid JSON formatting in API response.", err)
		return
	}

	for key, value := range json_map {
		switch key {

		case "stats":
			if data, ok := value.(map[string]interface{}); ok {
				temp_pool := Pool{}

				if temp_pool.BalancePaid, ok = data["paid"].(float64); ok {
					pool_balance_paid.With(prometheus.Labels{"pool": *poolURL}).Set(temp_pool.BalancePaid)
				} else {
					log.Println("WARN: Invalid JSON formatting in paid balance field.")
				}

				if temp_pool.BalanceUnpaid, ok = data["balance"].(float64); ok {
					pool_balance_unpaid.With(prometheus.Labels{"pool": *poolURL}).Set(temp_pool.BalanceUnpaid)
				} else {
					log.Println("WARN: Invalid JSON formatting in unpaid balance field.")
				}

				if temp_pool.BalanceUnconfirmed, ok = data["immature"].(float64); ok {
					pool_balance_unconfirmed.With(prometheus.Labels{"pool": *poolURL}).Set(temp_pool.BalanceUnconfirmed)
				} else {
					log.Println("WARN: Invalid JSON formatting in unconfirmed balance field.")
				}

				temp_rewards_total := temp_pool.BalancePaid + temp_pool.BalanceUnpaid + temp_pool.BalanceUnconfirmed
				pool_rewards_total := pool_info.BalancePaid + pool_info.BalanceUnpaid + pool_info.BalanceUnconfirmed

				if reward_diff := temp_rewards_total - pool_rewards_total; reward_diff >= 0 {
					pool_rewards.With(prometheus.Labels{"pool": *poolURL}).Add(reward_diff)
				} else {
					log.Println("WARN: Pool rewards decreased.", pool_rewards_total, "->", temp_rewards_total)
				}

				pool_info = temp_pool
			} else {
				log.Println("WARN: Invalid JSON formatting in statistics field.")
			}

		case "workers":
			if worker_list, ok := value.(map[string]interface{}); ok {
				for worker, worker_data := range worker_list {
					if data, ok := worker_data.(map[string]interface{}); ok {
						temp_worker := Worker{}

						if temp_worker.CurrentHashrate, ok = data["hr"].(float64); ok {
							pool_hashrate_current.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.CurrentHashrate)
						} else {
							log.Println("WARN: Invalid JSON formatting in worker current hashrate field.")
						}

						if temp_worker.AverageHashrate, ok = data["hr2"].(float64); ok {
							pool_hashrate_average.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.AverageHashrate)
						} else {
							log.Println("WARN: Invalid JSON formatting in worker average hashrate field.")
						}

						if temp_worker.ReportedHashrate, ok = data["rhr"].(float64); ok {
							pool_hashrate_reported.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.ReportedHashrate)
						} else {
							log.Println("WARN: Invalid JSON formatting in worker reported hashrate field.")
						}

						if temp_worker.SharesValid, ok = data["sharesValid"].(float64); ok {
							if share_diff := temp_worker.SharesValid - worker_info[worker].SharesValid; share_diff >= 0 {
								pool_shares_valid.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
							} else {
								log.Println("WARN: Valid shares decreased.", worker_info[worker].SharesValid, "->", temp_worker.SharesValid)
							}
						} else {
							log.Println("WARN: Invalid JSON formatting in worker valid shares field.")
						}

						if temp_worker.SharesInvalid, ok = data["sharesInvalid"].(float64); ok {
							if share_diff := temp_worker.SharesInvalid - worker_info[worker].SharesInvalid; share_diff >= 0 {
								pool_shares_invalid.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
							} else {
								log.Println("WARN: Invalid shares decreased.", worker_info[worker].SharesInvalid, "->", temp_worker.SharesInvalid)
							}
						} else {
							log.Println("WARN: Invalid JSON formatting in worker invalid shares field.")
						}

						if temp_worker.SharesStale, ok = data["sharesStale"].(float64); ok {
							if share_diff := temp_worker.SharesStale - worker_info[worker].SharesStale; share_diff >= 0 {
								pool_shares_stale.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
							} else {
								log.Println("WARN: Stale shares decreased.", worker_info[worker].SharesStale, "->", temp_worker.SharesStale)
							}
						} else {
							log.Println("WARN: Invalid JSON formatting in worker stale shares field.")
						}

						worker_info[worker] = temp_worker
					} else {
						log.Println("WARN: Invalid JSON formatting in worker data field.")
					}
				}
			} else {
				log.Println("WARN: Invalid JSON formatting in workers field.")
			}
		default:
		}
	}
}

func main() {
	flag.Parse()

	recordMetrics()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type Pool struct {
	BalancePaid        float64
	BalanceUnpaid      float64
	BalanceUnconfirmed float64
}

type Worker struct {
	CurrentHashrate  float64
	AverageHashrate  float64
	ReportedHashrate float64
	SharesValid      float64
	SharesInvalid    float64
	SharesStale      float64
}
