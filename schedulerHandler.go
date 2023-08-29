package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SchedulerService struct {
	SrvCtx               ServiceContext
	SrvDB                string
	scheduler_collection string
	project_collection   string
}

type TimeStamp struct {
	Payload   interface{} `json:"payload"`
	Month     string      `json:"month"`
	WeekDay   string      `json:"weekDay"`
	Day       string      `json:"day"`
	Hour      string      `json:"hour"`
	Minute    string      `json:"minute"`
	End       string      `json:"end"`
	Frequency int         `json:"frequency"`
	Date      string      `json:"date"`
}

func (schedulerService *SchedulerService) Bootstrap() {
	appRoute := schedulerService.SrvCtx.App.Group(viper.GetString("service.serviceURL"))
	appRoute.Use(ValidateToken())
	{
		appRoute.Post("/test_scheduler", schedulerService.DoSchedule)
		appRoute.Get("/test_scheduler", schedulerService.GetTimeStamp)
	}
}
func (schedulerService *SchedulerService) GetTimeStamp(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10000*time.Second)
	_ = ctx
	defer cancel()

	if c.Context().UserValue("AuthorizationRequired") == 1 {
		return c.Status(http.StatusUnauthorized).JSON(UserResponse{Status: http.StatusUnauthorized, Message: "error", Data: &fiber.Map{"data": "authorization has been denied for this request"}})
	}
	userIDValue := c.Context().UserValue("userid")
	var userID string
	if userIDValue != nil {
		userID = userIDValue.(string)
	}
	if userIDValue == nil {
		log.Error().Msg("User ID is nil")
	}
	// projectId := c.Params("projectId")
	schedulerCollection := schedulerService.SrvCtx.Db.Database(schedulerService.SrvDB).Collection(schedulerService.scheduler_collection)
	projectCollection := schedulerService.SrvCtx.Db.Database(schedulerService.SrvDB).Collection(schedulerService.project_collection)
	filter := bson.M{"status": bson.M{"$ne": "done"}, "userId": userID}
	cursor, err := schedulerCollection.Find(ctx, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	var results []bson.M
	if err := cursor.All(context.TODO(), &results); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}

	finalResult := make([]map[string]interface{}, 0)
	for _, singleResult := range results {
		dateArray := singleResult["scheduledAt"].(primitive.A)
		for _, date := range dateArray {
			testMap := make(map[string]interface{})
			dateTimeString := date.(string)
			layout := "2006-01-02 15:04:05"
			desiredTime, err := time.Parse(layout, dateTimeString)
			if err != nil {
				fmt.Println("Error parsing date and time:", err)
				return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
			}
			currentTime := time.Now()
			if desiredTime.Before(currentTime) {
				continue
			}
			testMap["date"] = date
			testMap["testName"] = singleResult["testName"]
			projectID := singleResult["projectId"].(string)
			objID, err := primitive.ObjectIDFromHex(projectID)
			projectFilter := bson.M{"_id": objID}
			var projectResult bson.M
			err = projectCollection.FindOne(ctx, projectFilter).Decode(&projectResult)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
			}
			testMap["projectName"] = projectResult["projectName"]
			finalResult = append(finalResult, testMap)
		}
	}
	return c.Status(http.StatusOK).JSON(UserResponse{Status: http.StatusOK, Message: "success", Data: finalResult})

}

// Schedule - Schedule a test
// POST /api/v2/scheduler/test_scheduler
// @Summary Schedule a test
// @Description Schedule a test to run on a given timestamp and given frequency
// @Tags         scheduler
// @security Authorization
// @Accept       json
// @Produce      json
// @Param        body body map[string]interface{} true "payload"
// @Success      200
// @Failure      401
// @Failure      500
// @Failure      422
// @Router       /api/v2/scheduler/test_scheduler [post]
func (schedulerService *SchedulerService) DoSchedule(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10000*time.Second)
	_ = ctx
	defer cancel()

	if c.Context().UserValue("AuthorizationRequired") == 1 {
		return c.Status(http.StatusUnauthorized).JSON(UserResponse{Status: http.StatusUnauthorized, Message: "error", Data: "authorization has been denied for this request"})
	}
	userIDValue := c.Context().UserValue("userid")
	var userID string
	if userIDValue != nil {
		userID = userIDValue.(string)
	}
	if userIDValue == nil {
		log.Error().Msg("User ID is nil")
	}
	var count = 0
	var timeStamp TimeStamp
	if err := c.BodyParser(&timeStamp); err != nil {
		return c.Status(http.StatusUnprocessableEntity).JSON(UserResponse{Status: http.StatusUnprocessableEntity, Message: "error", Data: err})
	}
	schedulerCollection := schedulerService.SrvCtx.Db.Database(schedulerService.SrvDB).Collection(schedulerService.scheduler_collection)

	localHour, err := strconv.Atoi(timeStamp.Hour)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	localMinute, err := strconv.Atoi(timeStamp.Minute)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	istTime := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), localHour, localMinute, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	utcTime := istTime.UTC()
	hour := strconv.Itoa(utcTime.Hour())
	minute := strconv.Itoa(utcTime.Minute())
	cronExpression := fmt.Sprintf("%s %s %s %s %s", minute, hour, timeStamp.Day, timeStamp.Month, timeStamp.WeekDay)
	localcronExpression := fmt.Sprintf("%s %s %s %s %s", timeStamp.Minute, timeStamp.Hour, timeStamp.Day, timeStamp.Month, timeStamp.WeekDay)
	schedulingDetails := make(map[string]interface{})
	payload := timeStamp.Payload.(map[string]interface{})
	schedulingDetails["projectId"] = payload["projectId"]
	schedulingDetails["testName"] = payload["testName"]
	schedulingDetails["cronExpression"] = cronExpression
	schedulingDetails["end"] = timeStamp.End
	schedulingDetails["frequency"] = timeStamp.Frequency
	//get scheduled dates
	dateArray := make([]string, 0)
	cronExp := localcronExpression
	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronSchedule, err := cronParser.Parse(cronExp)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	now := time.Now()
	nextTime := cronSchedule.Next(now)
	for i := 0; i < timeStamp.Frequency; i++ {
		dateArray = append(dateArray, nextTime.Format("2006-01-02 15:04:05"))
		nextTime = cronSchedule.Next(nextTime)
	}
	schedulingDetails["scheduledAt"] = dateArray
	schedulingDetails["date"] = timeStamp.Date
	schedulingDetails["createdAt"] = time.Now()
	schedulingDetails["status"] = ""
	schedulingDetails["updatedAt"] = time.Now()
	schedulingDetails["userId"] = userID
	result, err := schedulerCollection.InsertOne(ctx, schedulingDetails)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	insertId := result.InsertedID
	cr := cron.New()
	job, err := cr.AddFunc(localcronExpression, func() {
		if timeStamp.End != "never" {
			key := timeStamp.End
			switch key {
			case "date":
				{
					endDate := timeStamp.Date
					nowstr := fmt.Sprint(time.Now())
					if nowstr[0:10] == endDate {
						cr.Stop()
						fmt.Println("Scheduler stoped on given date")
						filter := bson.M{"_id": insertId.(primitive.ObjectID)}
						update := bson.M{"$set": bson.M{"status": "done"}}
						_, err := schedulerCollection.UpdateOne(context.Background(), filter, update)
						if err != nil {
							fmt.Println("error:", err)
							return
						}
						return
					}
				}
			case "after":
				{
					freq := timeStamp.Frequency
					if count == freq {
						cr.Stop()
						fmt.Println("Scheduler stoped after given frequency")
						filter := bson.M{"_id": insertId.(primitive.ObjectID)}
						update := bson.M{"$set": bson.M{"status": "done"}}
						_, err := schedulerCollection.UpdateOne(context.Background(), filter, update)
						if err != nil {
							fmt.Println("error:", err)
							return
						}
						return
					}
				}
			}
		}
		url := viper.GetString("goCustomTestURL")
		payloadBytes, err := json.Marshal(timeStamp.Payload)
		if err != nil {
			count += 1
			fmt.Println("Error encoding payload:", err)
			return
		}
		client := &http.Client{}
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
		if err != nil {
			count += 1
			fmt.Println("error during post request")
			return
		}
		req.Header.Add("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			count += 1
			fmt.Println("error while making post request to customtest", err)
			return
		}
		fmt.Println("status:", res.StatusCode)
		count += 1
		if count == timeStamp.Frequency {
			cr.Stop()
			fmt.Println("Scheduler stoped at the given frequency")
			filter := bson.M{"_id": insertId.(primitive.ObjectID)}
			update := bson.M{"$set": bson.M{"status": "done"}}
			_, err := schedulerCollection.UpdateOne(context.Background(), filter, update)
			if err != nil {
				fmt.Println("error:", err)
				return
			}
		}
		fmt.Println("frequency", count)
		defer res.Body.Close()
	})
	if err != nil {
		fmt.Println("Error adding cron job")
		return c.Status(http.StatusInternalServerError).JSON(UserResponse{Status: http.StatusInternalServerError, Message: "error", Data: err})
	}
	cr.Start()
	fmt.Println("Scheduler started", job)
	return c.JSON(map[string]string{"message": "scheduling done"})
}
