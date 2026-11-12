package computeclient

// Types for the compute internal API. They must match the pinned schema in
// openapi.json; contract_test.go fails the build when they drift (B6).
// A field without omitempty is required in the schema.

// ResolveRequest is the body of POST /internal/resolve.
type ResolveRequest struct {
	Name string `json:"name"`
}

// ResolveResponse is a resolved object name. The gateway returns it unchanged from GET /resolve.
type ResolveResponse struct {
	Name     string        `json:"name"`
	RA       float64       `json:"ra"`
	Dec      float64       `json:"dec"`
	PM       *ProperMotion `json:"pm"`       // null when unknown
	Parallax *float64      `json:"parallax"` // mas, null when unknown
	Type     *string       `json:"type"`     // null when unknown
	Links    []Link        `json:"links"`
}

// ProperMotion is in mas/yr.
type ProperMotion struct {
	PMRA  float64 `json:"pmra"`
	PMDec float64 `json:"pmdec"`
}

// Link points to an outside catalogue page.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// CoverageRequest is the body of POST /internal/coverage.
type CoverageRequest struct {
	RA  float64 `json:"ra"`
	Dec float64 `json:"dec"`
}

// CoverageResponse lists the SPHEREx epochs and releases covering a position.
type CoverageResponse struct {
	Covered  bool      `json:"covered"`
	Epochs   []Epoch   `json:"epochs"`
	Releases []Release `json:"releases"`
}

// Epoch is one SPHEREx pass over a position.
type Epoch struct {
	Index     int     `json:"index"`
	Date      string  `json:"date"`
	MJD       float64 `json:"mjd"`
	Detectors []int   `json:"detectors"`
}

// Release is a SPHEREx data release and the IRSA collection that holds it.
type Release struct {
	Name       string `json:"name"`
	Collection string `json:"collection"`
}

// ProcessRequest is the body of POST /internal/process.
type ProcessRequest struct {
	Params ViewParams `json:"params"`
	// OutputPrefix is the object storage prefix compute writes frame images under.
	OutputPrefix string `json:"output_prefix"`
	// CallbackURL receives POST {"progress": 0-100} while the job runs.
	CallbackURL string `json:"callback_url,omitempty"`
}

// ViewParams are the normalised view parameters. All of them except
// Target.Name feed the cache key (B5).
type ViewParams struct {
	Target      Target      `json:"target"`
	FOVArcmin   float64     `json:"fov_arcmin"`
	Mode        string      `json:"mode"`
	Detectors   []int       `json:"detectors"`
	Epochs      *DateRange  `json:"epochs,omitempty"`
	Difference  *Difference `json:"difference,omitempty"`
	Tracking    bool        `json:"tracking"`
	Stretch     *Stretch    `json:"stretch,omitempty"`
	Analysis    Analysis    `json:"analysis"`
	Collections []string    `json:"collections"`
}

// Target is an ICRS position in degrees.
type Target struct {
	RA   float64 `json:"ra"`
	Dec  float64 `json:"dec"`
	Name string  `json:"name,omitempty"`
}

// DateRange limits a view to epochs between two dates (YYYY-MM-DD).
type DateRange struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// Difference picks two epochs by index for mode=difference.
type Difference struct {
	A int `json:"a"`
	B int `json:"b"`
}

// Stretch is a display stretch. Type is asinh or log; percentiles are 0-100.
type Stretch struct {
	Type string  `json:"type"`
	PMin float64 `json:"pmin"`
	PMax float64 `json:"pmax"`
}

// Analysis toggles the optional analysis steps.
type Analysis struct {
	Sources    bool `json:"sources"`
	Crossmatch bool `json:"crossmatch"`
	SSOCheck   bool `json:"sso_check"`
}

// ProcessResponse is a rendered view.
type ProcessResponse struct {
	Epochs   []Epoch  `json:"epochs"`
	Frames   []Frame  `json:"frames"`
	Stretch  Stretch  `json:"stretch"`
	Warnings []string `json:"warnings"`
}

// Frame is a rendered frame: the CONTRACT Frame plus the object key of its PNG.
type Frame struct {
	FrameInfo
	ObjectKey string `json:"object_key"`
}

// FrameInfo is the CONTRACT Frame shape, as served to the client.
type FrameInfo struct {
	ID                      string   `json:"id"`
	EpochIndex              int      `json:"epoch_index"`
	Date                    string   `json:"date"`
	MJD                     float64  `json:"mjd"`
	Detector                int      `json:"detector"`
	WavelengthMinUM         float64  `json:"wavelength_min_um"`
	WavelengthMaxUM         float64  `json:"wavelength_max_um"`
	CenterRA                float64  `json:"center_ra"`
	CenterDec               float64  `json:"center_dec"`
	PixelScaleArcsec        float64  `json:"pixel_scale_arcsec"`
	Width                   int      `json:"width"`
	Height                  int      `json:"height"`
	OrientationDeg          float64  `json:"orientation_deg"`
	AlignmentResidualArcsec *float64 `json:"alignment_residual_arcsec"` // null when not measured
	Warnings                []string `json:"warnings"`
}

// ErrorBody is the CONTRACT error format, used by both services.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the inner part of ErrorBody.
type ErrorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}
