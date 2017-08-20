package diary

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	"github.com/gobuffalo/packr"
	"github.com/jinzhu/gorm"
	"github.com/matematik7/gongo"
	"github.com/matematik7/gongo/authorization"
	"github.com/matematik7/gongo/files"
	"github.com/matematik7/gongo/render"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/viper"

	"github.com/matematik7/camino-go/diary/models"
)

const PerPage = 10

type Diary struct {
	DB     *gorm.DB
	render *render.Render
	files  *files.Files
}

func New() *Diary {
	return &Diary{}
}

func (c *Diary) Configure(app gongo.App) error {
	c.DB = app["DB"].(*gorm.DB)
	c.render = app["Render"].(*render.Render)
	c.files = app["Files"].(*files.Files)

	c.render.AddTemplates(packr.NewBox("./templates"))

	c.render.AddContextFunc(func(r *http.Request, ctx render.Context) {
		var years []int
		// TODO: add error handling
		c.DB.Model(&models.DiaryEntry{}).
			Select("DISTINCT date_part('year', created_at) as year").
			Order("year desc").
			Pluck("year", &years)
		ctx["diaryYears"] = years
	})

	return nil
}

func (c Diary) Resources() []interface{} {
	return []interface{}{
		&models.DiaryEntry{},
		&models.Comment{},
		&models.EntryUserRead{},
	}
}

func (c Diary) CanEdit(entry models.DiaryEntry, userItf interface{}) bool {
	if userItf == nil {
		return false
	}
	user := userItf.(authorization.User)
	if user.HasPermissions("create_diary_entries") && entry.AuthorID == user.ID {
		return true
	}
	return user.HasPermissions("update_diary_entries")
}

func (c *Diary) ServeMux() http.Handler {
	router := chi.NewRouter()

	router.Get("/", c.ListHandler)

	router.Get("/read", c.ReadHandler)

	router.Route("/{diaryID:[0-9]+}", func(r chi.Router) {
		r.Get("/", c.ViewHandler)
		r.Post("/comment", c.CommentHandler)

		r.Route("/pictures", func(r chi.Router) {
			r.Get("/", c.PicturesHandler)
			r.Post("/", c.AddPictureHandler)

			// r.Route("/{imageID:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", func(r chi.Router) {
			r.Route("/{imageID:[0-9a-f]+}", func(r chi.Router) {
				r.Delete("/", c.DeletePictureHandler)
				r.Get("/delete", c.DeletePictureHandler)
			})
		})
	})

	return router
}

func (c *Diary) DeletePictureHandler(w http.ResponseWriter, r *http.Request) {
	diaryEntry := models.DiaryEntry{}
	query := c.DB.First(&diaryEntry, chi.URLParam(r, "diaryID"))
	if query.RecordNotFound() {
		// TODO: add not found to render
		c.render.Error(w, r, errors.New("Not found"))
		return
	} else if query.Error != nil {
		c.render.Error(w, r, query.Error)
		return
	}

	if !c.CanEdit(diaryEntry, r.Context().Value("user")) {
		// TODO: add forbidden to render
		c.render.Error(w, r, errors.New("Forbidden"))
		return
	}

	id, err := uuid.FromString(chi.URLParam(r, "imageID"))
	if err != nil {
		c.render.Error(w, r, err)
		return
	}

	image := files.Image{
		File: files.File{
			ID: id,
		},
	}

	if err := c.DB.Model(&diaryEntry).Association("Images").Delete(&image).Error; err != nil {
		c.render.Error(w, r, err)
		return
	}

	if err := c.files.Delete(image); err != nil {
		c.render.Error(w, r, err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/diary/%d/pictures", diaryEntry.ID), http.StatusFound)
}

func (c *Diary) AddPictureHandler(w http.ResponseWriter, r *http.Request) {
	diaryEntry := models.DiaryEntry{}
	query := c.DB.First(&diaryEntry, chi.URLParam(r, "diaryID"))
	if query.RecordNotFound() {
		// TODO: add not found to render
		c.render.Error(w, r, errors.New("Not found"))
		return
	} else if query.Error != nil {
		c.render.Error(w, r, query.Error)
		return
	}

	if !c.CanEdit(diaryEntry, r.Context().Value("user")) {
		// TODO: add forbidden to render
		c.render.Error(w, r, errors.New("Forbidden"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024)
	r.ParseMultipartForm(20 * 1024 * 1024)
	defer r.Body.Close()

	file, handler, err := r.FormFile("userfile")
	if err != nil {
		// TODO: use flashes for these errors that are form related
		c.render.Error(w, r, err)
		return
	}
	defer file.Close()

	img, err := c.files.NewImage(file, handler.Filename, r.FormValue("description"))
	if err != nil {
		c.render.Error(w, r, err)
		return
	}

	if err := c.DB.Model(&diaryEntry).Association("Images").Append(img).Error; err != nil {
		c.render.Error(w, r, err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/diary/%d/pictures", diaryEntry.ID), http.StatusFound)
}

func (c *Diary) PicturesHandler(w http.ResponseWriter, r *http.Request) {
	diaryEntry := models.DiaryEntry{}
	query := c.DB.Preload("Images").First(&diaryEntry, chi.URLParam(r, "diaryID"))
	if query.RecordNotFound() {
		// TODO: add not found to render
		c.render.Error(w, r, errors.New("Not found"))
		return
	} else if query.Error != nil {
		c.render.Error(w, r, query.Error)
		return
	}

	if !c.CanEdit(diaryEntry, r.Context().Value("user")) {
		// TODO: add forbidden to render
		c.render.Error(w, r, errors.New("Forbidden"))
		return
	}

	context := render.Context{
		"subpage": "Slike",
		"entry":   diaryEntry,
	}

	c.render.Template(w, r, "diary_pictures.html", context)
}

func (c *Diary) CommentHandler(w http.ResponseWriter, r *http.Request) {
	//TODO: move this get user to authorization to perform interface is nil validation and have proper context key
	userItf := r.Context().Value("user")
	if userItf == nil {
		//TODO: Add forbidden and not found to render
		// We DO need to see your identification.
		c.render.Error(w, r, errors.New("Forbidden"))
		return
	}
	user := userItf.(authorization.User)

	diaryEntry := models.DiaryEntry{}
	query := c.DB.First(&diaryEntry, chi.URLParam(r, "diaryID"))
	if query.RecordNotFound() {
		// TODO: add not found to render
		c.render.Error(w, r, errors.New("Not found"))
		return
	} else if query.Error != nil {
		c.render.Error(w, r, query.Error)
		return
	}

	comment := models.Comment{
		DiaryEntryID: diaryEntry.ID,
		AuthorID:     user.ID,
		Comment:      r.FormValue("comment"),
	}

	err := c.DB.Save(&comment).Error
	if err != nil {
		// TODO: figure better way for handling validation errors for bigger forms
		if _, ok := err.(govalidator.Errors); ok {
			// TODO: flash message about empty comment
			http.Redirect(w, r, r.Referer(), http.StatusFound)
		}
		c.render.Error(w, r, err)
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusFound)
}

func (c *Diary) ReadHandler(w http.ResponseWriter, r *http.Request) {
	userItf := r.Context().Value("user")
	if userItf == nil {
		//TODO: Add forbidden and not found to render
		c.render.Error(w, r, errors.New("Forbidden"))
		return
	}
	user := userItf.(authorization.User)

	// TODO mark everything as read on user register
	if err := c.DB.Model(&models.EntryUserRead{}).Where("user_id = ?", user.ID).Update("updated_at", "NOW()").Error; err != nil {
		c.render.Error(w, r, err)
		return
	}

	query := c.DB.Exec(
		`INSERT INTO entry_user_reads
		(created_at, updated_at, diary_entry_id, user_id)
		(
			SELECT NOW(), NOW(), id, ?
			FROM diary_entries
			LEFT JOIN (
				SELECT 1 as already_exists, diary_entry_id
				FROM entry_user_reads
				WHERE user_id = ?
			) ae ON ae.diary_entry_id = diary_entries.id
			WHERE ae.already_exists IS DISTINCT FROM 1
		)`,
		user.ID,
		user.ID,
	)
	if query.Error != nil {
		c.render.Error(w, r, query.Error)
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusFound)
}

func (c *Diary) ViewHandler(w http.ResponseWriter, r *http.Request) {
	var entry models.DiaryEntry

	query := c.DB.Preload("Author").
		Preload("Comments", func(db *gorm.DB) *gorm.DB {
			return db.Order("comments.created_at desc")
		}).
		Preload("Comments.Author").
		Preload("MapEntry.GpsData").
		Preload("Images").
		First(&entry, chi.URLParam(r, "diaryID"))

	if err := query.Error; err != nil {
		c.render.Error(w, r, err)
		return
	}

	// Mark as read if logged in
	if user := r.Context().Value("user"); user != nil {
		user := user.(authorization.User)

		entryUserRead := models.EntryUserRead{
			DiaryEntryID: entry.ID,
			UserID:       user.ID,
		}

		query := c.DB.First(&entryUserRead, entryUserRead)
		if !query.RecordNotFound() && query.Error != nil {
			c.render.Error(w, r, query.Error)
			return
		}

		if err := c.DB.Save(&entryUserRead).Error; err != nil {
			c.render.Error(w, r, err)
			return
		}
	}

	context := render.Context{
		"entry":       entry,
		"browser_key": viper.GetString("GMAP_BROWSER_KEY"),
		"CanEdit":     c.CanEdit,
	}

	c.render.Template(w, r, "diary_one.html", context)
}

func (c *Diary) ListHandler(w http.ResponseWriter, r *http.Request) {
	yearStr := r.URL.Query().Get("year")

	// show latest year on main page
	if r.URL.Path == "/" {
		var year []int
		query := c.DB.Model(&models.DiaryEntry{}).
			Select("DISTINCT date_part('year', created_at) as year").
			Order("year desc").
			Limit(1).
			Pluck("year", &year)
		if query.Error != nil {
			c.render.Error(w, r, query.Error)
			return
		}
		yearStr = strconv.Itoa(year[0])
	}

	query := c.DB.Model(&models.DiaryEntry{}).
		Select("*").
		Preload("Author").
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("diary_entry_id, RANDOM()").Select("distinct on (diary_entry_id) *")
		}).
		Order("created_at desc")

	var yearItf interface{}
	if yearStr != "" {
		year, err := strconv.Atoi(yearStr)
		if err != nil {
			c.render.Error(w, r, errors.Wrap(err, "invalid year"))
			return
		}
		query = query.Where("date_part('year', created_at) = ?", year)
		yearItf = year
	}

	// paging
	var count int
	if err := query.Count(&count).Error; err != nil {
		c.render.Error(w, r, err)
		return
	}
	pages := make([]int, (count+PerPage-1)/PerPage)
	for i := range pages {
		pages[i] = PerPage * i
	}

	query = query.Joins(
		`natural left join (
			SELECT diary_entry_id as id, count(*) as num_comments
			FROM comments
			WHERE comments.deleted_at IS NULL
			GROUP BY diary_entry_id
		) c`,
	)

	// get new status for user if logged in
	if user := r.Context().Value("user"); user != nil {
		user := user.(authorization.User)

		query = query.Joins(
			`natural left join (
				SELECT
					entry_user_reads.diary_entry_id as id,
					true as viewed,
					rc.created_at > entry_user_reads.updated_at as new_comments
				FROM entry_user_reads
				LEFT JOIN (
					SELECT diary_entry_id, MAX(created_at) as created_at
					FROM comments
					WHERE comments.deleted_at IS NULL
					GROUP BY diary_entry_id
				) rc ON rc.diary_entry_id = entry_user_reads.diary_entry_id
				WHERE user_id = ?
			) r`,
			user.ID,
		)
	} else {
		query = query.Select("*, true as viewed")
	}

	offset := 0
	if r.URL.Query().Get("offset") != "" {
		var err error
		offset, err = strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			c.render.Error(w, r, errors.Wrap(err, "invalid offset"))
			return
		}
		query = query.Offset(offset)
	}
	nextOffset := offset + PerPage
	prevOffset := offset - PerPage

	var entries []models.DiaryEntry
	if err := query.Limit(PerPage).Find(&entries).Error; err != nil {
		c.render.Error(w, r, err)
		return
	}

	context := render.Context{
		"entries":    entries,
		"offset":     offset,
		"pages":      pages,
		"prevOffset": prevOffset,
		"nextOffset": nextOffset,
		"year":       yearItf,
	}

	c.render.Template(w, r, "diary_all.html", context)
}
