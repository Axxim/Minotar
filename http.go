package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/minotar/minecraft"
)

type Router struct {
	Mux *mux.Router
}

// Middleware function to manipulate our request and response.
func imgdHandler(router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding")
		router.ServeHTTP(w, r)
	})
}

type NotFoundHandler struct{}

// Handles 404 errors
func (h NotFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "404 not found")
	log.Infof("%s %s 404", r.RemoteAddr, r.RequestURI)
}

// GetWidth converts and sanitizes the string for the avatar width.
func (router *Router) GetWidth(inp string) uint {
	out64, err := strconv.ParseUint(inp, 10, 0)
	out := uint(out64)
	if err != nil {
		return DefaultWidth
	} else if out > MaxWidth {
		return MaxWidth
	} else if out < MinWidth {
		return MinWidth
	}
	return out

}

// SkinPage shows only the user's skin.
func (router *Router) SkinPage(w http.ResponseWriter, r *http.Request) {
	stats.Requested("Skin")
	vars := mux.Vars(r)
	username := vars["username"]
	skin := fetchSkin(username)

	if r.Header.Get("If-None-Match") == skin.Skin.Hash {
		w.WriteHeader(http.StatusNotModified)
		log.Infof("%s %s 304 %s", r.RemoteAddr, r.RequestURI, skin.Skin.Source)
		return
	}

	w.Header().Add("Cache-Control", fmt.Sprintf("public, max-age=%d", config.Server.Ttl))
	w.Header().Add("ETag", skin.Hash)
	w.Header().Add("Content-Type", "image/png")
	skin.WriteSkin(w)
	log.Infof("%s %s 200 %s", r.RemoteAddr, r.RequestURI, skin.Skin.Source)
}

// DownloadPage shows the skin and tells the browser to attempt to download it.
func (router *Router) DownloadPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Disposition", "attachment; filename=\"skin.png\"")
	router.SkinPage(w, r)
}

// ResolveMethod pulls the Get<resource> method from the skin. Originally this used
// reflection, but that was slow.
func (router *Router) ResolveMethod(skin *mcSkin, resource string) func(int) error {
	switch resource {
	case "Avatar":
		return skin.GetHead
	case "Helm":
		return skin.GetHelm
	case "Cube":
		return skin.GetCube
	case "Bust":
		return skin.GetBust
	case "Body":
		return skin.GetBody
	case "Armor/Bust":
		return skin.GetArmorBust
	case "Armour/Bust":
		return skin.GetArmorBust
	case "Armor/Body":
		return skin.GetArmorBody
	case "Armour/Body":
		return skin.GetArmorBody
	default:
		return skin.GetHelm
	}
}

func (router *Router) getResizeMode(ext string) string {
	switch ext {
	case ".svg":
		return "None"
	default:
		return "Normal"
	}
}

func (router *Router) writeType(ext string, skin *mcSkin, w http.ResponseWriter) {
	w.Header().Add("Cache-Control", fmt.Sprintf("public, max-age=%d", config.Server.Ttl))
	w.Header().Add("ETag", skin.Hash)
	switch ext {
	case ".svg":
		w.Header().Add("Content-Type", "image/svg+xml")
		skin.WriteSVG(w)
	default:
		w.Header().Add("Content-Type", "image/png")
		skin.WritePNG(w)
	}
}

// Serve binds the route and makes a handler function for the requested resource.
func (router *Router) Serve(resource string) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		width := router.GetWidth(vars["width"])
		skin := fetchSkin(vars["username"])
		skin.Mode = router.getResizeMode(vars["extension"])
		stats.Requested(resource)

		if r.Header.Get("If-None-Match") == skin.Skin.Hash {
			w.WriteHeader(http.StatusNotModified)
			log.Infof("%s %s 304 %s", r.RemoteAddr, r.RequestURI, skin.Skin.Source)
			return
		}

		err := router.ResolveMethod(skin, resource)(int(width))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "500 internal server error")
			log.Infof("%s %s 500 %s", r.RemoteAddr, r.RequestURI, skin.Skin.Source)
			stats.Errored("InternalServerError")
			return
		}
		router.writeType(vars["extension"], skin, w)
		log.Infof("%s %s 200 %s", r.RemoteAddr, r.RequestURI, skin.Skin.Source)
	}

	router.Mux.HandleFunc("/"+strings.ToLower(resource)+"/{username:"+minecraft.ValidUsernameRegex+"}{extension:(?:\\..*)?}", fn)
	router.Mux.HandleFunc("/"+strings.ToLower(resource)+"/{username:"+minecraft.ValidUsernameRegex+"}/{width:[0-9]+}{extension:(?:\\..*)?}", fn)
}

// Bind routes to the ServerMux.
func (router *Router) Bind() {

	router.Mux.NotFoundHandler = NotFoundHandler{}

	router.Serve("Avatar")
	router.Serve("Helm")
	router.Serve("Cube")
	router.Serve("Bust")
	router.Serve("Body")
	router.Serve("Armor/Bust")
	router.Serve("Armour/Bust")
	router.Serve("Armor/Body")
	router.Serve("Armour/Body")

	router.Mux.HandleFunc("/download/{username:"+minecraft.ValidUsernameRegex+"}{extension:(?:.png)?}", router.DownloadPage)
	router.Mux.HandleFunc("/skin/{username:"+minecraft.ValidUsernameRegex+"}{extension:(?:.png)?}", router.SkinPage)

	router.Mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s\n", ImgdVersion)
		log.Infof("%s %s 200", r.RemoteAddr, r.RequestURI)
	})

	router.Mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(stats.ToJSON())
		log.Infof("%s %s 200", r.RemoteAddr, r.RequestURI)
	})

	router.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, config.Server.URL, http.StatusFound)
		log.Infof("%s %s 200", r.RemoteAddr, r.RequestURI)
	})
}

func fetchSkin(username string) *mcSkin {
	if username == "char" || username == "MHF_Steve" {
		skin, _ := minecraft.FetchSkinForSteve()
		return &mcSkin{Skin: skin}
	}

	if cache.has(strings.ToLower(username)) {
		stats.HitCache()
		return &mcSkin{Processed: nil, Skin: cache.pull(strings.ToLower(username))}
	}

	uuid, err := minecraft.NormalizePlayerForUUID(username)
	if err != nil {
		log.Debugf("Failed UUID lookup: %s (%s)", username, err.Error())
		skin, _ := minecraft.FetchSkinForSteve()
		stats.Errored("LookupUUID")
		return &mcSkin{Skin: skin}
	}

	skin, err := minecraft.FetchSkinUUID(uuid)
	if err != nil {
		log.Debugf("Failed Skin SessionProfile: %s (%s)", username, err.Error())
		// Let's fallback to S3 and try and serve at least an old skin...
		skin, err = minecraft.FetchSkinUsernameS3(username)
		if err != nil {
			log.Debugf("Failed Skin S3: %s (%s)", username, err.Error())
			// Well, looks like they don't exist after all.
			skin, _ = minecraft.FetchSkinForSteve()
			stats.Errored("FallbackSteve")
		} else {
			stats.Errored("FallbackUsernameS3")
		}
	}

	stats.MissCache()
	cache.add(strings.ToLower(username), skin)

	return &mcSkin{Processed: nil, Skin: skin}
}
