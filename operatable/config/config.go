package config

type values struct {
	App struct {
		Port string
	}
	System struct {
		DoneCallbackURL string
	}
}

var v values

func init() {
	// if slices.Contains([]string{"", "local"}, os.Getenv("ENV")) {
	// 	err := godotenv.Load(".env.local")
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }

	// // app
	// v.App.Port = os.Getenv("PORT")
	v.App.Port = "8070"
	v.System.DoneCallbackURL = "http://operator-operator-rest-api.app.svc.cluster.local:8070/api/jobs"
}

func Get() *values {
	return &v
}
