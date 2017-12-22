package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ContenderController struct {
	db *sql.DB
}

// ServeHTTP routes incoming requests to the right service.
func (cc *ContenderController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := new(Contender)
	ServeResource(w, r, cc, c)
}

// Path returns the URL extension associated with the Contender resource.
func (cc *ContenderController) Path() string {
	return "/contenders/"
}

// DBTableName returns the table name for Contenders.
func (cc *ContenderController) DBTableName() string {
	return "contenders"
}

// Create writes a new contender to the db for each given Resource.
func (cc *ContenderController) Create(m []Resource) ([]int, *ApplicationError) {
	// Create a slice of Contender pointers by asserting on a slice of Resources interfaces
	var contenders []*Contender
	for i := 0; i < len(m); i++ {
		c := m[i]
		contenders = append(contenders, c.(*Contender))
	}

	// Create the SQL query
	// todo: %s and cc.DBTableName() instead?
	// todo: time.Now() instead of CURRENT_TIMESTAMP?
	// todo: check other query statements
	q := `
	INSERT INTO contenders (
		fb_id, fb_group_id,
		name, total_posts, avg_likes_per_post, total_likes_received, total_likes_given, posts_used,
		created_at, updated_at
	) values (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	// Begin sql transaction
	tx, err := cc.db.Begin()
	if err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}
	defer tx.Rollback()

	// Insert each Contender into contenders table
	var contenderIds []int
	// todo: use range?
	for i := 0; i < len(contenders); i++ {
		c := contenders[i]

		posts := slicePostIdsToStringPosts(c.TotalPosts)
		postsUsed := slicePostIdsToStringPosts(c.PostsUsed)

		result, err := tx.Exec(q,
			c.FbId, c.FbGroupId,
			c.Name, posts, c.AvgLikesPerPost, c.TotalLikesReceived, c.TotalLikesGiven, postsUsed)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// Save each Id to return
		id, err := result.LastInsertId()
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}
		contenderIds = append(contenderIds, int(id))
	}

	// Commit sql transaction
	if err = tx.Commit(); err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}

	return contenderIds, nil
}

// Read returns the contender in the db for a given FbId.
func (cc *ContenderController) Read(fbId int) (Resource, *ApplicationError) {
	log.Println("Read: Contender", fbId)

	// todo: better way to shorten line of code and reuse in ReadCollection?
	var fbGroupId int
	var name string
	var totalPostsString string
	var avgLikesPerPost float64
	var totalLikesReceived int
	var totalLikesGiven int
	var postsUsedString string
	var createdAt time.Time
	var updatedAt time.Time

	// Grab contender entry from table
	q := fmt.Sprintf("SELECT * FROM contenders WHERE fb_id=%d", fbId)
	err := cc.db.QueryRow(q).Scan(&fbId, &fbGroupId, &name, &totalPostsString, &avgLikesPerPost, &totalLikesReceived,
		&totalLikesGiven, &postsUsedString, &createdAt, &updatedAt) // todo: okay to unscan into fbId arg?
	switch {
	case err == sql.ErrNoRows:
		msg := fmt.Sprintf("Couldn't find any resource with id: %d", fbId)
		return nil, &ApplicationError{Msg: msg, Code: http.StatusNotFound}
	case err != nil:
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}

	// todo: better way to abstract unloading strings of ints and creating individual contender (and ReadCollection)?
	// Split comma separated strings to slices of ints
	totalPosts, err := stringPostsToSlicePostIds(totalPostsString)
	if err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}
	postsUsed, err := stringPostsToSlicePostIds(postsUsedString)
	if err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}

	// Create Contender
	c := Contender{
		FbId:               fbId,
		FbGroupId:          fbGroupId,
		Name:               name,
		TotalPosts:         totalPosts,
		AvgLikesPerPost:    float64(avgLikesPerPost),
		TotalLikesReceived: totalLikesReceived,
		TotalLikesGiven:    totalLikesGiven,
		PostsUsed:          postsUsed,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}

	return &c, nil
}

// Update writes the db column value for each variable Contender parameter.
//
// Writes TotalPosts, AvgLikesPerPost, TotalLikesReceived, TotalLikesGiven, PostsUsed, and UpdatedAt.
// todo: test when fb_id does not exist
func (cc *ContenderController) Update(m []Resource) *ApplicationError {
	// Create a slice of Contender pointers by asserting on a slice of Resources interfaces
	var contenders []*Contender
	for i := 0; i < len(m); i++ {
		c := m[i]
		contenders = append(contenders, c.(*Contender))
	}

	// Begin sql transaction
	tx, err := cc.db.Begin()
	if err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}
	defer tx.Rollback()

	// Create the SQL query
	q := `
	UPDATE contenders SET
		total_posts=?, avg_likes_per_post=?, total_likes_received=?, total_likes_given=?, posts_used=?,
		updated_at=CURRENT_TIMESTAMP
		WHERE fb_id=?
	`

	// Iterate through each contender and update it in the db
	for _, c := range contenders {
		posts := slicePostIdsToStringPosts(c.TotalPosts)
		postsUsed := slicePostIdsToStringPosts(c.PostsUsed)

		res, err := tx.Exec(q, posts, c.AvgLikesPerPost, c.TotalLikesReceived, c.TotalLikesGiven, postsUsed, c.FbId)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// Not really sure what this can error on
		numrows, err := res.RowsAffected()
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// If more or less than one row is affected then we have a problem
		switch {
		case numrows == 0:
			msg := fmt.Sprintf("Couldn't find any resource to update with id: %d", c.FbId)
			return &ApplicationError{Msg: msg, Code: http.StatusNotFound}
		case numrows != 1:
			// This is really bad, should never see. May be an SQL injection attempt.
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}
	}

	// Commit sql transaction
	if err = tx.Commit(); err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}

	return nil
}

// Destroy deletes any given Id from the db.
func (cc *ContenderController) Destroy(ids []int) *ApplicationError {
	// Begin sql transaction
	tx, err := cc.db.Begin()
	if err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}
	defer tx.Rollback()

	// Create the SQL query
	q := fmt.Sprintf("DELETE FROM %s WHERE fb_id = $1;", cc.DBTableName())

	// Iterate through each contender and update it in the db
	for _, v := range ids {
		// todo: a lot of repeated code from update's error handling
		res, err := tx.Exec(q, v)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// Not really sure what this can error on
		numrows, err := res.RowsAffected()
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// If more or less than one row is affected then we have a problem
		switch {
		case numrows == 0:
			msg := fmt.Sprintf("Couldn't find any resource to destroy with id: %d", v)
			return &ApplicationError{Msg: msg, Code: http.StatusNotFound}
		case numrows != 1:
			// This is really bad, should never see. May be an SQL injection attempt.
			msg := "Something is wrong with our database - we'll be back up soon!"
			return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}
	}

	// Commit sql transaction
	if err = tx.Commit(); err != nil {
		msg := "Something is wrong with our database - we'll be back up soon!"
		return &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}

	return nil
}

// ReadCollection returns all Contenders in the db.
func (cc *ContenderController) ReadCollection() ([]Resource, *ApplicationError) {
	log.Println("Read collection: Contenders")

	// Grab contender entries from table
	rows, err := cc.db.Query("SELECT * FROM contenders")
	switch {
	case err == sql.ErrNoRows:
		msg := fmt.Sprintf("None of that kind of resource here!")
		return nil, &ApplicationError{Msg: msg, Code: http.StatusNoContent}
	case err != nil:
		msg := "Something is wrong with our database - we'll be back up soon!"
		return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
	}
	defer rows.Close()

	// Create a Contender from each row
	contenders := make([]Resource, 0) // Container for the Resources we're about to return
	for rows.Next() {
		var fbId int
		var fbGroupId int
		var name string
		var totalPostsString string
		var avgLikesPerPost float64
		var totalLikesReceived int
		var totalLikesGiven int
		var postsUsedString string
		var createdAt time.Time
		var updatedAt time.Time

		err := rows.Scan(&fbId, &fbGroupId, &name, &totalPostsString, &avgLikesPerPost,
			&totalLikesReceived, &totalLikesGiven, &postsUsedString, &createdAt, &updatedAt)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		// Split comma separated strings to slices of ints
		totalPosts, err := stringPostsToSlicePostIds(totalPostsString)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}
		postsUsed, err := stringPostsToSlicePostIds(postsUsedString)
		if err != nil {
			msg := "Something is wrong with our database - we'll be back up soon!"
			return nil, &ApplicationError{Msg: msg, Err: err, Code: http.StatusInternalServerError}
		}

		c := Contender{
			FbId:               fbId,
			FbGroupId:          fbGroupId,
			Name:               name,
			TotalPosts:         totalPosts,
			AvgLikesPerPost:    float64(avgLikesPerPost),
			TotalLikesReceived: totalLikesReceived,
			TotalLikesGiven:    totalLikesGiven,
			PostsUsed:          postsUsed,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		}

		contenders = append(contenders, &c)
	}

	return contenders, nil
}

// /////
// Non API methods and helper functions
// todo: does this section belong?
// /////

func (cc *ContenderController) PopulateContendersTable() error {
	log.Println("Attempting to create Contenders")

	// Convert contender struct pointers into a slice of Resource interfaces
	contenders, err := PullContendersFromFb()
	contenderResources := make([]Resource, len(contenders))
	for i, v := range contenders {
		contenderResources[i] = Resource(v)
	}

	if err != nil {
		log.Println("Failed to get Contenders from fb:", err)
		return err
	}

	_, err = cc.Create(contenderResources)
	if err != nil {
		log.Println("Failed to create Contenders from FB:", err)
		return err
	}

	log.Println("Successfully created Contenders")
	return nil
}

// stringPostsToSlicePostIds is a helper function that converts a string of ints to a slice of ints.
//
// "1, 2, 3" to []int{1, 2, 3}
// "1,2,3" will throw an error
// returns []int{} if given string is ""
func stringPostsToSlicePostIds(s string) ([]int, error) {
	stringSlice := strings.Split(s, ", ")

	var intSlice []int
	if stringSlice[0] != "" {
		intSlice = make([]int, len(stringSlice))
		for i, v := range stringSlice {
			s, err := strconv.Atoi(v)
			if err != nil {
				log.Println("Failed to convert string of ints to slice:", err)
				return nil, err
			}
			intSlice[i] = s
		}
	}
	return intSlice, nil
}

// slicePostIdsToStringPosts is a helper function that converts a slice of post ids to a string of ids.
//
// todo: recover() from panic()? is this possible?
// todo: probably a better way to do this...
// https://stackoverflow.com/questions/25025467/catching-panics-in-golang
func slicePostIdsToStringPosts(slicePostIds []int) string {
	stringPosts := fmt.Sprint(slicePostIds)                     // [1 2 3] to "[1 2 3]"
	splitStringPosts := strings.Split(stringPosts, " ")         // "[1 2 3]" to ["[1 2 3]"]
	joinedStringPosts := strings.Join(splitStringPosts, ", ")   // ["[1 2 3]"] to "[1, 2, 3]"
	trimmedStringPosts := strings.Trim(joinedStringPosts, "[]") // "[1, 2, 3]" to "1, 2, 3"

	return trimmedStringPosts
}
