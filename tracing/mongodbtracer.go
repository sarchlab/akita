package tracing

import (
	"context"
	"log"
	"time"

	"github.com/rs/xid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBTracer is a tracer that can dump the tasks into a MongoDB database.
type MongoDBTracer struct {
	clientSide   *mongo.Client
	collect      *mongo.Collection
	uri          string
	tracingTasks map[string]Task
}

// SetURI sets the server and the port to connect to
func (t *MongoDBTracer) SetURI(uri string) {
	t.uri = uri
}

// Init connects to the MongoDB database.
func (t *MongoDBTracer) Init() {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.clientSide, err = mongo.Connect(ctx, options.Client().ApplyURI(t.uri))
	if err != nil {
		log.Panic(err)
	}

	dbName := xid.New().String()
	log.Printf("Trace is Collected in Database: %s\n", dbName)

	t.collect = t.clientSide.Database(dbName).Collection("trace")

	t.createIndexes()
}

// Create the indexes for the database
func (t *MongoDBTracer) createIndexes() {
	t.createIndex("id", true)
	t.createIndex("parentid", true)
	t.createIndex("kind", true)
	t.createIndex("what", true)
	t.createIndex("where", true)
	t.createIndex("starttime", false)
	t.createIndex("endtime", false)
	t.createIndex("detail", true)
}

// Update records in Database by converting to BSON so db can read
func (t *MongoDBTracer) createIndex(key string, useHash bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var value interface{}
	if useHash {
		value = "hashed"
	} else {
		value = 1
	}

	_, err := t.collect.Indexes().CreateOne(ctx,
		mongo.IndexModel{
			Keys: bson.D{bson.E{Key: key, Value: value}},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}

// StartTask marks the start of a task.
func (t *MongoDBTracer) StartTask(task Task) {
	t.tracingTasks[task.ID] = task
}

// StepTask marks a milestone during the executing of a task.
func (t *MongoDBTracer) StepTask(task Task) {
	// Do nothing for now
}

// EndTask writes the task into the database.
func (t *MongoDBTracer) EndTask(task Task) {
	originalTask := t.tracingTasks[task.ID]
	originalTask.EndTime = task.EndTime
	originalTask.Detail = nil
	delete(t.tracingTasks, task.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := t.collect.InsertOne(ctx, originalTask)
	if err != nil {
		log.Panic(err)
	}
}

// NewMongoDBTracer returns a new MongoDBTracer
func NewMongoDBTracer() *MongoDBTracer {
	t := &MongoDBTracer{
		uri:          "mongodb://localhost:27017",
		tracingTasks: make(map[string]Task),
	}
	return t
}
