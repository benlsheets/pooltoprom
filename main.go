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
)

func recordMetrics() {
	go func() {
		var account_info Account
		worker_info := make(map[string]Worker)

		requestURL := fmt.Sprintf("https://%s/api/accounts/%s", *poolURL, *wallet)
		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("accept", "application/json")

		for {
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Fatal(err)
			}

			resBody, err := ioutil.ReadAll(res.Body)
			if err != nil {
				log.Fatal(err)
			}

			var f interface{}
			err = json.Unmarshal(resBody, &f)
			if err != nil {
				log.Fatal(err)
			}

			m := f.(map[string]interface{})

			for key, value := range m {
				switch key {
				case "rewards":
					// fmt.Println(key, "\n", value)

				case "stats":
					data := value.(map[string]interface{})

					account_info.BalancePaid = data["paid"].(float64) / ScalingFactor
					pool_balance_paid.With(prometheus.Labels{"pool": *poolURL}).Set(account_info.BalancePaid)

					account_info.BalanceUnpaid = data["balance"].(float64) / ScalingFactor
					pool_balance_unpaid.With(prometheus.Labels{"pool": *poolURL}).Set(account_info.BalanceUnpaid)

					account_info.BalanceUnconfirmed = data["immature"].(float64) / ScalingFactor
					pool_balance_unconfirmed.With(prometheus.Labels{"pool": *poolURL}).Set(account_info.BalanceUnconfirmed)

				case "workers":
					worker_list := value.(map[string]interface{})

					for worker, worker_data := range worker_list {
						data := worker_data.(map[string]interface{})

						temp_worker := Worker{}
						temp_worker.CurrentHashrate = data["hr"].(float64)
						temp_worker.AverageHashrate = data["hr2"].(float64)
						temp_worker.ReportedHashrate = data["rhr"].(float64)
						temp_worker.SharesValid = data["sharesValid"].(float64)
						temp_worker.SharesInvalid = data["sharesInvalid"].(float64)
						temp_worker.SharesStale = data["sharesStale"].(float64)

						pool_hashrate_current.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.CurrentHashrate)

						pool_hashrate_average.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.AverageHashrate)

						pool_hashrate_reported.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Set(temp_worker.ReportedHashrate)

						if share_diff := temp_worker.SharesValid - worker_info[worker].SharesValid; share_diff >= 0 {
							pool_shares_valid.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
						} else {
							log.Println("WARN: Valid shares decreased.", worker_info[worker].SharesValid, "->", temp_worker.SharesValid)
						}

						if share_diff := temp_worker.SharesInvalid - worker_info[worker].SharesInvalid; share_diff >= 0 {
							pool_shares_invalid.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
						} else {
							log.Println("WARN: Invalid shares decreased.", worker_info[worker].SharesInvalid, "->", temp_worker.SharesInvalid)
						}

						if share_diff := temp_worker.SharesStale - worker_info[worker].SharesStale; share_diff >= 0 {
							pool_shares_stale.With(prometheus.Labels{"pool": *poolURL, "worker": worker}).Add(share_diff)
						} else {
							log.Println("WARN: Stale shares decreased.", worker_info[worker].SharesStale, "->", temp_worker.SharesStale)
						}

						worker_info[worker] = temp_worker

						time.Sleep(1 * time.Minute)
					}
				default:
				}
			}
		}
	}()
}

var (
	pool_balance_paid = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_paid",
		Help: "Balance paid from pool",
	},
		[]string{"pool"},
	)

	pool_balance_unpaid = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_unpaid",
		Help: "Unpaid balance on pool",
	},
		[]string{"pool"},
	)

	pool_balance_unconfirmed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_balance_unconfirmed",
		Help: "Unconfirmed balance on pool",
	},
		[]string{"pool"},
	)

	pool_hashrate_current = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_current",
		Help: "Current Worker Hashrates",
	},
		[]string{"pool", "worker"},
	)

	pool_hashrate_average = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_average",
		Help: "Average Worker Hashrates",
	},
		[]string{"pool", "worker"},
	)

	pool_hashrate_reported = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pool_hashrate_reported",
		Help: "Reported Worker Hashrates",
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
)

func main() {
	flag.Parse()

	recordMetrics()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type Account struct {
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

const ScalingFactor float64 = 1e9
