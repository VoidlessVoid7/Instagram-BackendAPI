package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"insta-api/helper"
	"insta-api/models"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)

	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func Encrypt(key, data []byte) ([]byte, error) {
	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func Decrypt(key, data []byte) ([]byte, error) {
	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, err
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

type PostFilter struct {
	LimitedPosts []models.Post `json:"Posts"`
	LowerId      string        `json:"lowerId"`
}

var postCollection = helper.ConnectPostsDB()
var userCollection = helper.ConnectUsersDB()

var key, _ = GenerateKey()

// POST ENDPOINTS

func GetPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var post models.Post
	postId := strings.TrimPrefix(r.URL.Path, "/api/posts/")
	id, _ := primitive.ObjectIDFromHex(postId)
	filter := bson.M{"_id": id}
	err := postCollection.FindOne(context.TODO(), filter).Decode(&post)
	if err != nil {
		helper.GetError(err, w)
		return
	}
	json.NewEncoder(w).Encode(post)
}

func CreatePost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var post models.Post
	_ = json.NewDecoder(r.Body).Decode(&post)
	post.Timestamp = time.Now()
	result, err := postCollection.InsertOne(context.TODO(), post)
	if err != nil {
		helper.GetError(err, w)
		return
	}
	json.NewEncoder(w).Encode(result)
}

// USER ENDPOINTS

func CreateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var user models.User
	_ = json.NewDecoder(r.Body).Decode(&user)

	// HASHING
	passwd := []byte(user.Password)
	ciphertext, err := Encrypt(key, passwd)
	user.Password = hex.EncodeToString(ciphertext)
	// HASHING COMPLETE

	result, err := userCollection.InsertOne(context.TODO(), user)
	if err != nil {
		helper.GetError(err, w)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var user models.User
	userId := strings.TrimPrefix(r.URL.Path, "/api/users/")
	id, _ := primitive.ObjectIDFromHex(userId)
	filter := bson.M{"_id": id}
	err := userCollection.FindOne(context.TODO(), filter).Decode(&user)
	if err != nil {
		helper.GetError(err, w)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func GetPostsByUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var posts []models.Post
	var user models.User
	userId := strings.TrimPrefix(r.URL.Path, "/api/posts/users/")
	userId = strings.Split(userId, "?")[0]
	query := r.URL.Query()
	limit, _ := strconv.ParseInt(query["limit"][0], 10, 64)
	lowerId := query["lowerid"]
	id, _ := primitive.ObjectIDFromHex(userId)
	filter := bson.M{"_id": id}
	err := userCollection.FindOne(context.TODO(), filter).Decode(&user)
	cur, err := postCollection.Find(context.TODO(), bson.M{"author": user.Name})
	if err != nil {
		helper.GetError(err, w)
		return
	}
	i := 0
	var retId string
	defer cur.Close(context.TODO())
	for cur.Next(context.TODO()) {
		var post models.Post
		err := cur.Decode(&post)
		if err != nil {
			log.Fatal(err)
		}
		if len(lowerId) > 0 {
			if strings.Compare(post.ID.Hex(), lowerId[0]) == 1 {
				posts = append(posts, post)
				i += 1
			}
		} else {
			posts = append(posts, post)
			i += 1
		}
		retId = post.ID.Hex()
		if int64(i) == limit {
			break
		}
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	retObj := PostFilter{LimitedPosts: posts, LowerId: retId}
	ret, err := json.Marshal(retObj)
	if err != nil {
		log.Println(err)
	}
	fmt.Fprintf(w, string(ret))
}

func main() {

	http.HandleFunc("/api/posts", CreatePost)
	http.HandleFunc("/api/posts/", GetPost)
	http.HandleFunc("/api/posts/users/", GetPostsByUser)

	http.HandleFunc("/api/users", CreateUser)
	http.HandleFunc("/api/users/", GetUser)

	log.Fatal(http.ListenAndServe(":8000", nil))
}
