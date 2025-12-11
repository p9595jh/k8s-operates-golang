package config

type values struct {
	App struct {
		Port string
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
}

func Get() *values {
	return &v
}
