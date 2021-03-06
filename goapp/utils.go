/*
 * Copyright (c) 2013 Matt Jibson <matt.jibson@gmail.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package goapp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"appengine"
	"appengine/blobstore"
	aimage "appengine/image"
	"appengine/urlfetch"
	"appengine/user"
	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	mpg "github.com/MiniProfiler/go/miniprofiler_gae"
	"github.com/mjibson/goon"
	"goapp/atom"
	"goapp/rdf"
	"goapp/rss"
	"goapp/sanitizer"
)

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

type Includes struct {
	Angular             string
	BootstrapCss        string
	BootstrapJs         string
	FontAwesome         string
	Jquery              string
	JqueryUI            string
	Underscore          string
	MiniProfiler        template.HTML
	User                *User
	Messages            []string
	GoogleAnalyticsId   string
	GoogleAnalyticsHost string
	IsDev               bool
	IsAdmin             bool
	StripeKey           string
	StripePlans         []Plan
}

var (
	Angular      string
	BootstrapCss string
	BootstrapJs  string
	FontAwesome  string
	Jquery       string
	JqueryUI     string
	Underscore   string
	isDevServer  bool
)

func init() {
	angular_ver := "1.0.7"
	bootstrap_ver := "2.3.2"
	font_awesome_ver := "3.2.1"
	jquery_ver := "1.9.1"
	jqueryui_ver := "1.10.3"
	underscore_ver := "1.4.4"
	isDevServer = appengine.IsDevAppServer()

	if appengine.IsDevAppServer() {
		Angular = fmt.Sprintf("/static/js/angular-%v.js", angular_ver)
		BootstrapCss = fmt.Sprintf("/static/css/bootstrap-combined-no-icons-%v.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("/static/js/bootstrap-%v.js", bootstrap_ver)
		FontAwesome = fmt.Sprintf("/static/css/font-awesome-%v.css", font_awesome_ver)
		Jquery = fmt.Sprintf("/static/js/jquery-%v.js", jquery_ver)
		JqueryUI = fmt.Sprintf("/static/js/jquery-ui-%v.js", jqueryui_ver)
		Underscore = fmt.Sprintf("/static/js/underscore-%v.js", underscore_ver)
	} else {
		Angular = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/angularjs/%v/angular.min.js", angular_ver)
		BootstrapCss = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/css/bootstrap-combined.no-icons.min.css", bootstrap_ver)
		BootstrapJs = fmt.Sprintf("//netdna.bootstrapcdn.com/twitter-bootstrap/%v/js/bootstrap.min.js", bootstrap_ver)
		FontAwesome = fmt.Sprintf("//netdna.bootstrapcdn.com/font-awesome/%v/css/font-awesome.min.css", font_awesome_ver)
		Jquery = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jquery/%v/jquery.min.js", jquery_ver)
		JqueryUI = fmt.Sprintf("//ajax.googleapis.com/ajax/libs/jqueryui/%v/jquery-ui.min.js", jqueryui_ver)
		Underscore = fmt.Sprintf("/static/js/underscore-%v.min.js", underscore_ver)
	}
}

func includes(c mpg.Context, w http.ResponseWriter, r *http.Request) *Includes {
	i := &Includes{
		Angular:             Angular,
		BootstrapCss:        BootstrapCss,
		BootstrapJs:         BootstrapJs,
		FontAwesome:         FontAwesome,
		Jquery:              Jquery,
		JqueryUI:            JqueryUI,
		Underscore:          Underscore,
		MiniProfiler:        c.Includes(r),
		GoogleAnalyticsId:   GOOGLE_ANALYTICS_ID,
		GoogleAnalyticsHost: GOOGLE_ANALYTICS_HOST,
		IsDev:               isDevServer,
		StripeKey:           STRIPE_KEY,
		StripePlans:         STRIPE_PLANS,
	}

	if cu := user.Current(c); cu != nil {
		gn := goon.FromContext(c)
		user := &User{Id: cu.ID}
		if err := gn.Get(user); err == nil {
			i.User = user
			i.IsAdmin = cu.Admin

			if len(user.Messages) > 0 {
				i.Messages = user.Messages
				user.Messages = nil
				gn.Put(user)
			}

			/*
				if _, err := r.Cookie("update-bug"); err != nil {
					i.Messages = append(i.Messages, "Go Read had some problems updating feeds. It may take a while for new stories to appear again. Sorry about that.")
					http.SetCookie(w, &http.Cookie{
						Name: "update-bug",
						Value: "done",
						Expires: time.Now().Add(time.Hour * 24 * 7),
					})
				}
			*/
		}
	}

	return i
}

var dateFormats = []string{
	"01-02-2006",
	"01/02/2006",
	"01/02/2006 - 15:04",
	"01/02/2006 15:04:05 MST",
	"01/02/2006 3:04 PM",
	"02-01-2006",
	"02/01/2006",
	"02.01.2006 -0700",
	"02/01/2006 - 15:04",
	"02.01.2006 15:04",
	"02/01/2006 15:04:05",
	"02.01.2006 15:04:05",
	"02-01-2006 15:04:05 MST",
	"02 Jan 2006",
	"02 Jan 2006 15:04:05",
	"02 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 MST",
	"02 Jan 2006 15:04:05 UT",
	"02 Jan 2006 15:04 MST",
	"06-1-2 15:04",
	"06/1/2 15:04",
	"1/2/2006",
	"1/2/2006 15:04:05 MST",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 3:04:05 PM MST",
	"15:04 02.01.2006 -0700",
	"2006-01-02",
	"2006/01/02",
	"2006-01-02 00:00:00.0 15:04:05.0 -0700",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05-0700",
	"2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05Z",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05:00",
	"2006-01-02T15:04:05 -0700",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05:-0700",
	"2006-01-02T15:04:05-07:00:00",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04-07:00",
	"2006-01-02T15:04Z",
	"2006-1-02T15:04:05Z",
	"2006-1-2",
	"2006-1-2 15:04:05",
	"2006-1-2T15:04:05Z",
	"2006 January 02",
	"2-1-2006",
	"2/1/2006",
	"2.1.2006 15:04:05",
	"2 Jan 2006",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 Z",
	"2 January 2006",
	"2 January 2006 15:04:05 -0700",
	"2 January 2006 15:04:05 MST",
	"6-1-2 15:04",
	"6/1/2 15:04",
	"Jan 02, 2006",
	"Jan 02 2006 03:04:05PM",
	"Jan 2, 2006",
	"Jan 2, 2006 15:04:05 MST",
	"Jan 2, 2006 3:04:05 PM",
	"Jan 2, 2006 3:04:05 PM MST",
	"January 02, 2006",
	"January 02, 2006 03:04 PM",
	"January 02, 2006 15:04",
	"January 02, 2006 15:04:05 MST",
	"January 2, 2006",
	"January 2, 2006 03:04 PM",
	"January 2, 2006 15:04:05",
	"January 2, 2006 15:04:05 MST",
	"January 2, 2006, 3:04 p.m.",
	"January 2, 2006 3:04 PM",
	"Mon, 02 Jan 06 15:04:05 MST",
	"Mon, 02 Jan 2006",
	"Mon, 02 Jan 2006 15:04:05",
	"Mon, 02 Jan 2006 15:04:05 00",
	"Mon, 02 Jan 2006 15:04:05 -07",
	"Mon 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 --0700",
	"Mon, 02 Jan 2006 15:04:05 -07:00",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon,02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 GMT-0700",
	"Mon , 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05MST",
	"Mon, 02 Jan 2006 15:04:05 MST -0700",
	"Mon, 02 Jan 2006 15:04:05 MST-07:00",
	"Mon, 02 Jan 2006 15:04:05 UT",
	"Mon, 02 Jan 2006 15:04:05 Z",
	"Mon, 02 Jan 2006 15:04 -0700",
	"Mon, 02 Jan 2006 15:04 MST",
	"Mon,02 Jan 2006 15:04 MST",
	"Mon, 02 Jan 2006 3:04:05 PM MST",
	"Mon, 02 January 2006",
	"Mon,02 January 2006 14:04:05 MST",
	"Mon, 2006-01-02 15:04",
	"Mon, 2 Jan 06 15:04:05 -0700",
	"Mon, 2 Jan 06 15:04:05 MST",
	"Mon, 2 Jan 15:04:05 MST",
	"Mon, 2 Jan 2006",
	"Mon,2 Jan 2006",
	"Mon, 2 Jan 2006 15:04",
	"Mon, 2 Jan 2006 15:04:05",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05-0700",
	"Mon, 2 Jan 2006 15:04:05 -0700 MST",
	"mon,2 Jan 2006 15:04:05 MST",
	"Mon 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05MST",
	"Mon, 2 Jan 2006 15:04:05 UT",
	"Mon, 2 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 2006, 15:04 -0700",
	"Mon, 2 Jan 2006 15:04 MST",
	"Mon, 2, Jan 2006 15:4",
	"Mon, 2 Jan 2006 15:4:5 -0700 GMT",
	"Mon, 2 Jan 2006 15:4:5 MST",
	"Mon, 2 Jan 2006 3:04:05 PM -0700",
	"Mon, 2 January 2006 15:04:05 -0700",
	"Mon, 2 January 2006 15:04:05 MST",
	"Mon, 2 January 2006, 15:04:05 MST",
	"Mon, 2 January 2006, 15:04 -0700",
	"Mon, 2 January 2006 15:04 MST",
	"Monday, 02 January 2006 15:04:05",
	"Monday, 02 January 2006 15:04:05 -0700",
	"Monday, 02 January 2006 15:04:05 MST",
	"Monday, 2 Jan 2006 15:04:05 -0700",
	"Monday, 2 Jan 2006 15:04:05 MST",
	"Monday, 2 January 2006 15:04:05 -0700",
	"Monday, 2 January 2006 15:04:05 MST",
	"Monday, January 02, 2006",
	"Monday, January 2, 2006",
	"Monday, January 2, 2006 03:04 PM",
	"Monday, January 2, 2006 15:04:05 MST",
	"Mon Jan 02 2006 15:04:05 -0700",
	"Mon, Jan 02,2006 15:04:05 MST",
	"Mon Jan 02, 2006 3:04 pm",
	"Mon Jan 2 15:04:05 2006 MST",
	"Mon Jan 2 15:04 2006",
	"Mon, Jan 2 2006 15:04:05 -0700",
	"Mon, Jan 2 2006 15:04:05 -700",
	"Mon, Jan 2, 2006 15:04:05 MST",
	"Mon, January 02, 2006 15:04:05 MST",
	"Mon, January 02, 2006, 15:04:05 MST",
	"Mon, January 2 2006 15:04:05 -0700",
	time.ANSIC,
	time.RFC1123,
	time.RFC1123Z,
	time.RFC3339,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RubyDate,
	time.UnixDate,
	"Updated January 2, 2006",
}

const dateFormatCount = 500

func parseDate(c appengine.Context, feed *Feed, ds ...string) (t time.Time, err error) {
	for _, d := range ds {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		for _, f := range dateFormats {
			if t, err = time.Parse(f, d); err == nil {
				return
			}
		}
		gn := goon.FromContext(c)
		df := &DateFormat{
			Id:    rand.Int63n(dateFormatCount),
			Feed:  feed.Url,
			Value: d,
		}
		gn.Put(df)
	}
	err = fmt.Errorf("could not parse date: %v", strings.Join(ds, ", "))
	return
}

func ParseFeed(c appengine.Context, u string, b []byte) (*Feed, []*Story, error) {
	f := Feed{Url: u}
	var s []*Story

	a := atom.Feed{}
	var atomerr, rsserr, rdferr, err error
	var fb, eb *url.URL
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if atomerr = d.Decode(&a); atomerr == nil {
		f.Title = a.Title
		if t, err := parseDate(c, &f, string(a.Updated)); err == nil {
			f.Updated = t
		}

		if fb, err = url.Parse(a.XMLBase); err != nil {
			fb, _ = url.Parse("")
		}
		if len(a.Link) > 0 {
			f.Link = findBestAtomLink(c, a.Link).Href
			if l, err := fb.Parse(f.Link); err == nil {
				f.Link = l.String()
			}
		}

		for _, i := range a.Entry {
			if eb, err = fb.Parse(i.XMLBase); err != nil {
				eb = fb
			}
			st := Story{
				Id:    i.ID,
				Title: i.Title,
			}
			if t, err := parseDate(c, &f, string(i.Updated)); err == nil {
				st.Updated = t
			}
			if t, err := parseDate(c, &f, string(i.Published)); err == nil {
				st.Published = t
			}
			if len(i.Link) > 0 {
				st.Link = findBestAtomLink(c, i.Link).Href
				if l, err := eb.Parse(st.Link); err == nil {
					st.Link = l.String()
				}
			}
			if i.Author != nil {
				st.Author = i.Author.Name
			}
			if i.Content != nil {
				if len(strings.TrimSpace(i.Content.Body)) != 0 {
					st.content = i.Content.Body
				} else if len(i.Content.InnerXML) != 0 {
					st.content = i.Content.InnerXML
				}
			} else if i.Summary != nil {
				st.content = i.Summary.Body
			}
			s = append(s, &st)
		}

		return parseFix(c, &f, s)
	}

	r := rss.Rss{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	d.DefaultSpace = "DefaultSpace"
	if rsserr = d.Decode(&r); rsserr == nil {
		f.Title = r.Title
		f.Link = r.Link
		if t, err := parseDate(c, &f, r.LastBuildDate, r.PubDate); err == nil {
			f.Updated = t
		} else {
			c.Warningf("no rss feed date: %v", f.Link)
		}

		for _, i := range r.Items {
			st := Story{
				Link:   i.Link,
				Author: i.Author,
			}
			if i.Title != "" {
				st.Title = i.Title
			} else if i.Description != "" {
				i.Title = i.Description
			}
			if i.Content != "" {
				st.content = i.Content
			} else if i.Title != "" && i.Description != "" {
				st.content = i.Description
			}
			if i.Guid != nil {
				st.Id = i.Guid.Guid
			}
			if i.Enclosure != nil && strings.HasPrefix(i.Enclosure.Type, "audio/") {
				st.MediaContent = i.Enclosure.Url
			} else if i.Media != nil && strings.HasPrefix(i.Media.Type, "audio/") {
				st.MediaContent = i.Media.URL
			}
			if t, err := parseDate(c, &f, i.PubDate, i.Date, i.Published); err == nil {
				st.Published = t
				st.Updated = t
			}

			s = append(s, &st)
		}

		return parseFix(c, &f, s)
	}

	rd := rdf.RDF{}
	d = xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = charset.NewReader
	if rdferr = d.Decode(&rd); rdferr == nil {
		if rd.Channel != nil {
			f.Title = rd.Channel.Title
			f.Link = rd.Channel.Link
			if t, err := parseDate(c, &f, rd.Channel.Date); err == nil {
				f.Updated = t
			}
		}

		for _, i := range rd.Item {
			st := Story{
				Id:     i.About,
				Title:  i.Title,
				Link:   i.Link,
				Author: i.Creator,
			}
			if len(i.Description) > 0 {
				st.content = html.UnescapeString(i.Description)
			} else if len(i.Content) > 0 {
				st.content = html.UnescapeString(i.Content)
			}
			if t, err := parseDate(c, &f, i.Date); err == nil {
				st.Published = t
				st.Updated = t
			}
			s = append(s, &st)
		}

		return parseFix(c, &f, s)
	}

	c.Warningf("atom parse error: %s", atomerr.Error())
	c.Warningf("xml parse error: %s", rsserr.Error())
	c.Warningf("rdf parse error: %s", rdferr.Error())
	return nil, nil, fmt.Errorf("Could not parse feed data")
}

func findBestAtomLink(c appengine.Context, links []atom.Link) atom.Link {
	getScore := func(l atom.Link) int {
		switch {
		case l.Rel == "hub":
			return 0
		case l.Rel == "alternate" && l.Type == "text/html":
			return 4
		case l.Type == "text/html":
			return 3
		case l.Rel != "self":
			return 2
		default:
			return 1
		}
	}

	var bestlink atom.Link
	bestscore := -1
	for _, l := range links {
		score := getScore(l)
		if score > bestscore {
			bestlink = l
			bestscore = score
		}
	}

	return bestlink
}

func parseFix(c appengine.Context, f *Feed, ss []*Story) (*Feed, []*Story, error) {
	g := goon.FromContext(c)
	f.Checked = time.Now()
	fk := g.Key(f)
	f.Image = loadImage(c, f)
	f.Link = strings.TrimSpace(f.Link)
	f.Title = html.UnescapeString(f.Title)

	if u, err := url.Parse(f.Url); err == nil {
		if ul, err := u.Parse(f.Link); err == nil {
			f.Link = ul.String()
		}
	}
	base, err := url.Parse(f.Link)
	if err != nil {
		c.Warningf("unable to parse link: %v", f.Link)
	}

	var nss []*Story
	for _, s := range ss {
		s.Parent = fk
		s.Created = f.Checked
		s.Link = strings.TrimSpace(s.Link)
		if !s.Updated.IsZero() && s.Published.IsZero() {
			s.Published = s.Updated
		}
		if s.Published.IsZero() || f.Checked.Before(s.Published) {
			s.Published = f.Checked
		}
		if !s.Updated.IsZero() {
			s.Date = s.Updated.Unix()
		} else {
			s.Date = s.Published.Unix()
		}
		if s.Id == "" {
			if s.Link != "" {
				s.Id = s.Link
			} else if s.Title != "" {
				s.Id = s.Title
			} else {
				c.Errorf("story has no id: %v", s)
				return nil, nil, fmt.Errorf("Bad item data in feed")
			}
		}
		// if a story doesn't have a link, see if its id is a URL
		if s.Link == "" {
			if u, err := url.Parse(s.Id); err == nil {
				s.Link = u.String()
			}
		}
		if base != nil && s.Link != "" {
			link, err := base.Parse(s.Link)
			if err == nil {
				s.Link = link.String()
			} else {
				c.Warningf("unable to resolve link: %v", s.Link)
			}
		}
		const keySize = 500
		sk := g.Key(s)
		if kl := len(sk.String()); kl > keySize {
			c.Warningf("key too long: %v, %v, %v", kl, f.Url, s.Id)
			continue
		}
		su, serr := url.Parse(s.Link)
		if serr != nil {
			su = &url.URL{}
			s.Link = ""
		}
		const snipLen = 100
		s.content, s.Summary = sanitizer.Sanitize(s.content, su)
		s.Summary = sanitizer.SnipText(s.Summary, snipLen)
		s.Title = html.UnescapeString(sanitizer.StripTags(s.Title))
		nss = append(nss, s)
	}

	return f, nss, nil
}

func loadImage(c appengine.Context, f *Feed) string {
	s := f.Link
	if s == "" {
		s = f.Url
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	u.Path = "/favicon.ico"
	u.RawQuery = ""
	u.Fragment = ""

	g := goon.FromContext(c)
	i := &Image{Id: u.String()}
	if err := g.Get(i); err == nil {
		return i.Url
	}
	client := urlfetch.Client(c)
	r, err := client.Get(u.String())
	if err != nil || r.StatusCode != http.StatusOK || r.ContentLength == 0 {
		return ""
	}
	b, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return ""
	}
	buf := bytes.NewBuffer(b)
	_, t, err := image.DecodeConfig(buf)
	if err != nil {
		t = "application/octet-stream"
	} else {
		t = "image/" + t
	}
	w, err := blobstore.Create(c, t)
	if err != nil {
		return ""
	}
	if _, err := w.Write(b); err != nil {
		return ""
	}
	if w.Close() != nil {
		return ""
	}
	i.Blob, _ = w.Key()
	su, err := aimage.ServingURL(c, i.Blob, &aimage.ServingURLOptions{Size: 16})
	if err != nil {
		return ""
	}
	i.Url = su.String()
	g.Put(i)
	return i.Url
}

func updateAverage(f *Feed, previousUpdate time.Time, updateCount int) {
	if previousUpdate.IsZero() || updateCount < 1 {
		return
	}

	// if multiple updates occurred, assume they were evenly spaced
	interval := time.Since(previousUpdate) / time.Duration(updateCount)

	// rather than calculate a strict mean, we weight
	// each new interval, gradually decaying the influence
	// of older intervals
	old := float64(f.Average) * (1.0 - NewIntervalWeight)
	cur := float64(interval) * NewIntervalWeight
	f.Average = time.Duration(old + cur)
}

const notViewedDisabled = oldDuration + time.Hour*24*7

var timeMax time.Time = time.Date(3000, time.January, 1, 0, 0, 0, 0, time.UTC)

func scheduleNextUpdate(f *Feed) {
	if f.NotViewed() {
		f.NextUpdate = timeMax
		return
	}

	now := time.Now()
	if f.Date.IsZero() {
		f.NextUpdate = now.Add(UpdateDefault)
		return
	}

	// calculate the delay until next check based on average time between updates
	pause := time.Duration(float64(f.Average) * UpdateFraction)

	// if we have never found an update, start with a default wait time
	if pause == 0 {
		pause = UpdateDefault
	}

	// if it has been much longer than expected since the last update,
	// gradually reduce the frequency of checks
	since := time.Since(f.Date)
	if since > pause*UpdateLongFactor {
		pause = time.Duration(float64(since) / UpdateLongFactor)
	}

	// enforce some limits
	if pause < UpdateMin {
		pause = UpdateMin
	}
	if pause > UpdateMax {
		pause = UpdateMax
	}

	// introduce a little random jitter to break up
	// convoys of updates
	jitter := time.Duration(rand.Int63n(int64(UpdateJitter)))
	if rand.Intn(2) == 0 {
		pause += jitter
	} else {
		pause -= jitter
	}
	f.NextUpdate = time.Now().Add(pause)
}
