package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	io "io/ioutil"
	"net/http"
	"os"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v7"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	// "github.com/gofiber/fiber/v2/middleware/csrf"
	// "github.com/gofiber/fiber/v2/middleware/limiter"
	swagger "github.com/arsmn/fiber-swagger/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServiceContext for all the services
type ServiceContext struct {
	Port   string
	Db     *mongo.Client
	Logger zerolog.Logger
	App    *fiber.App
	Cache  *elasticsearch.Client
	UserDB *gorm.DB
}

// SECRET -  DO NOT CHANGE - any change will mean you cannot decrypt the values already encrypted
var SECRET = "123abcdefghijklm456nopqrstuvwxyz"

// Load the configuration from config file and load the data into a json object
func loadConfig() {
	// Config is stored as Base64 encoded string. Decoding config

	cfg, err := io.ReadFile(".config")
	if err != nil {
		panic(err)
	}
	cfgData, err := base64.StdEncoding.DecodeString(string(cfg))
	if err != nil {
		panic(err)
	}
	// Read the config data from the decoded json string
	viper.SetConfigType("json")
	if err = viper.ReadConfig(bytes.NewReader(cfgData)); err != nil {
		panic(err)
	}
}

// @title Swagger for Artemis Scheduler api
// @version 2.0
// @description List of APIs for Artemis  Scheduler
// @host
// @BasePath /
// @Schemes http https
func main() {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, NoColor: false})
	//TODO - below log statement should be written to log steam like loki, elastic
	zlog := zerolog.New(os.Stderr).With().Timestamp().Logger()

	//TODO - Check license to see if the service can run
	log.Info().Msg("license check")

	// Read service configuration
	loadConfig()
	log.Info().Msg("=== Starting " + viper.GetString("service.name"))

	// setup router
	app := fiber.New()

	// middleware initiation
	Middleware(app)

	// Set the router port - where to run API
	port := viper.GetString("service.port")

	// setup database
	db, err := ConnectDB()
	log.Info().Msg("database Connected ")

	if err != nil {
		log.Error().Err(err).Msg("error in database connection:")
		panic(err)
	}

	defer db.Disconnect(context.Background())

	/* Connect to the postgres user database */
	ucdsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		viper.GetString("pgContentDB.user"),
		viper.GetString("pgContentDB.password"),
		viper.GetString("pgContentDB.host"),
		viper.GetString("pgContentDB.port"),
		viper.GetString("pgContentDB.database"),
	)
	// ucdsn := "postgres://" + viper.GetString("pgContentDB.user") + ":" + viper.GetString("pgContentDB.password") + "@" + viper.GetString("DB_SERVER") + ":" + viper.GetString("DB_PORT") + "/wk_user_management"
	fmt.Printf("dsn %s", ucdsn)
	app.Use(dbMiddleware(ucdsn))

	app.Get("/project/swagger/*", swagger.HandlerDefault) // default

	//service context
	configServiceContext := &ServiceContext{Db:db,Logger: zlog, Port: port, App: app}

	schedulerService := SchedulerService{SrvCtx: *configServiceContext,
		scheduler_collection: viper.GetString("serviceScheduler.scheduler_collection"),
		project_collection: viper.GetString("serviceScheduler.artemis_projects"),
		SrvDB:                viper.GetString("mongodb.database")}
	schedulerService.Bootstrap()

	app.Get("/metrics", monitor.New(monitor.Config{Title: "Content service Metrics Page"}))

	// start
	app.Listen(":" + port)

}

// ConnectDB to connect database
func ConnectDB() (*mongo.Client, error) {
	// client, err := mongo.NewClient(options.Client().ApplyURI(fmt.Sprintf("mongodb://%s",
	// // 	viper.GetString("mongodb.user"),
	// // 	viper.GetString("mongodb.password"),
	//  	viper.GetString("mongodb.host"))))
	// // 	viper.GetString("mongodb.database")) + "?retryWrites=true&w=majority&authSource=admin"))
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	// client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://artemis:engroartemis@3.108.215.86:27017/?authMechanism=SCRAM-SHA-1"))
	// client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://artemis:engroartemis@3.108.215.86:27017/?authMechanism=SCRAM-SHA-1"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// Middleware for cors, csrf, limiter
func Middleware(app *fiber.App) {
	// middleware for cors handling
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "POST, GET, PUT, DELETE",
		AllowHeaders:     "Access-Control-Allow-Origin, Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization",
		AllowCredentials: true,
		ExposeHeaders:    "Content-Length",
		MaxAge:           86400,
	}))

	// middleware for logging requests
	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{
		// For more options, see the Config section
		Next:         nil,
		Done:         nil,
		Format:       "[${time}] ${status} - ${latency} ${method} ${path} - ${reqHeaders} - ${ip} - ${host} - ${resBody}\n",
		TimeFormat:   "15:04:05",
		TimeZone:     "Local",
		TimeInterval: 500 * time.Millisecond,
		Output:       os.Stdout,
	}))
}

func dbMiddleware(dbStr string) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {

		fmt.Println(dbStr)
		db, err := gorm.Open("postgres", dbStr)
		fmt.Printf("db:%+v", db)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: &fiber.Map{"data": "Failed to connect with DB"}})
		}

		defer db.Close()
		db.DB().SetMaxIdleConns(10)
		db.LogMode(true)
		db.SingularTable(true)

		// Set the connection in the context
		c.Context().SetUserValue("UDB", db)

		// Proceed to the next middleware or route handler
		return c.Next()
	}
}
