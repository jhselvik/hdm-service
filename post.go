package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	fb "github.com/huandu/facebook"
	"log"
	"time"
)

type Post struct {
	Id         string `facebook:",required"`
	PostedDate string
	Author     string
	Likes      []Like
	CreatedAt  *time.Time
	UpdatedAt  *time.Time
}

type Like struct {
	Id   string
	Name string
}

func (p *Post) DBTableName() string {
	return "posts"
}

func (p *Post) Path() string {
	return "/posts/"
}

func (p *Post) CreatePost(tx *sql.Tx) (int64, error) {
	q := `
	INSERT INTO posts (
		Id,
		PostedDate,
		Author,
		Likes,
		CreatedAt,
		UpdatedAt
	) values (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	likes, err := json.Marshal(p.Likes)
	if err != nil {
		return 0, err
	}

	result, err := tx.Exec(q, p.Id, p.PostedDate, p.Author, likes, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// asumes never goesdown?
func (p *Post) updateLikes(n int) {

}

// CreatePostsTable creates the posts table if it does not exist
func CreatePostsTable(startDate time.Time, db *sql.DB) error {
	q := `
	CREATE TABLE IF NOT EXISTS posts(
		Id TEXT NOT NULL,
		PostedDate DATETIME,
		Author TEXT,
		Likes BLOB,
		CreatedAt DATETIME,
		UpdatedAt DATETIME
	);
	`
	_, err := db.Exec(q)
	if err != nil {
		log.Println("Failed to CREATE posts table")
		return err
	}

	session := GetFBSession()
	fbPosts, err := GetFBPosts(startDate, session)
	if err != nil {
		log.Fatal("Failed to get posts from facebook")
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		log.Println("Failed to BEGIN txn:", err)
		return err
	}
	defer tx.Rollback()

	for i := 0; i < len(fbPosts); i++ {
		// Create each Post in DB
		// todo: should this check if post already exists?
		_, err := fbPosts[i].CreatePost(tx)
		if err != nil {
			log.Printf("Failed to create post")
			return err
		}

		// for each like, give a likes given
		db := GetDBHandle()
		for j := 0; j < len(fbPosts[i].Likes); j++ {
			c, err := GetContenderByUsername(db, fbPosts[i].Likes[j].Name)
			if err != nil {
				log.Println("Failed to get contender", fbPosts[i].Likes[j].Name)
				return err
			}
			err = c.updateTotalLikesGiven(tx)
			if err != nil {
				log.Println("Failed to update totalLikesGiven for contender", fbPosts[i].Likes[j].Name)
				return err
			}
		}
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		log.Println("Failed to COMMIT txn:", err)
		return err
	}

	return nil
}

func GetFBPosts(startDate time.Time, session *fb.Session) ([]Post, error) {
	// Get the group feed
	response, err := fb.Get(fmt.Sprintf("/%s/feed", GetGroupID()), fb.Params{
		"access_token": GetAccessToken(),
		"feilds":       []string{"from", "created_time"},
	})
	if err != nil {
		log.Fatal("Error requesting group feed")
		return nil, err
	}

	// Get the feed's paging object
	paging, err := response.Paging(session)
	if err != nil {
		log.Fatal("Error generating the feed response Paging object")
		return nil, err
	}

	var posts []Post
	count := 1

	// loop until a fb post's created_time is older than startDate
Loop:
	for {
		results := paging.Data()
		log.Println("Posts page ", count)

		// 25 posts per page, load data into a Post struct
		for i := 0; i < len(results); i++ {
			var p Post
			facebookPost := fb.Result(results[i]) // cast the var

			// stop when post reaches startDate
			p.PostedDate = facebookPost.Get("created_time").(string)
			t, err := time.Parse(GoTimeLayout, p.PostedDate)
			if err != nil {
				log.Fatal("Failed to parse post's postedDate")
				return nil, err
			}
			if t.Before(startDate) {
				log.Println("Reached a post before the startDate")
				break Loop
			}

			p.Id = facebookPost.Get("id").(string)
			p.Author = facebookPost.Get("from.name").(string)
			p.PostedDate = t.String()

			// unload Likes data into a Like struct
			if facebookPost.Get("likes.data") != nil {
				var likes []Like
				numLikes := facebookPost.Get("likes.data").([]interface{})
				for j := 0; j < len(numLikes); j++ {
					var l Like
					l.Id = numLikes[j].(map[string]interface{})["id"].(string)
					l.Name = numLikes[j].(map[string]interface{})["name"].(string)
					likes = append(likes, l)
				}
				p.Likes = likes

			} else {
				p.Likes = nil
			}

			// save the new Post
			posts = append(posts, p)
		}

		noMore, err := paging.Next()
		if err != nil {
			log.Fatal("Error accessing Response page's Next object")
			return nil, err
		}
		if noMore {
			log.Println("Reached the end of group feed")
			break Loop
		}
		count++
	}
	log.Println("Number posts:", len(posts))
	return posts, nil
}
