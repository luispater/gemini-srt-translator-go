package languages

// LanguageMap contains mappings from language names to language codes
var LanguageMap = map[string]string{
	// Major World Languages
	"arabic":     "ar",
	"chinese":    "zh",
	"english":    "en",
	"french":     "fr",
	"german":     "de",
	"hindi":      "hi",
	"italian":    "it",
	"japanese":   "ja",
	"korean":     "ko",
	"portuguese": "pt",
	"russian":    "ru",
	"spanish":    "es",

	// Chinese Variants
	"simplified chinese":  "chs",
	"traditional chinese": "cht",
	"mandarin":            "chs",
	"cantonese":           "zh-hk",

	// European Languages
	"albanian":       "sq",
	"basque":         "eu",
	"belarusian":     "be",
	"bosnian":        "bs",
	"bulgarian":      "bg",
	"catalan":        "ca",
	"croatian":       "hr",
	"czech":          "cs",
	"czech language": "cs",
	"check":          "cs",
	"danish":         "da",
	"dutch":          "nl",
	"estonian":       "et",
	"finnish":        "fi",
	"flemish":        "nl",
	"galician":       "gl",
	"georgian":       "ka",
	"greek":          "el",
	"hungarian":      "hu",
	"icelandic":      "is",
	"irish":          "ga",
	"latvian":        "lv",
	"lithuanian":     "lt",
	"luxembourgish":  "lb",
	"macedonian":     "mk",
	"maltese":        "mt",
	"montenegrin":    "me",
	"norwegian":      "no",
	"polish":         "pl",
	"romanian":       "ro",
	"serbian":        "sr",
	"slovak":         "sk",
	"slovene":        "sl",
	"slovenian":      "sl",
	"swedish":        "sv",
	"ukrainian":      "uk",
	"welsh":          "cy",

	// Middle Eastern & African Languages
	"afrikaans":   "af",
	"amharic":     "am",
	"armenian":    "hy",
	"azerbaijani": "az",
	"farsi":       "fa",
	"persian":     "fa",
	"hebrew":      "he",
	"kurdish":     "ku",
	"pashto":      "ps",
	"somali":      "so",
	"swahili":     "sw",
	"tigrinya":    "ti",
	"yoruba":      "yo",
	"zulu":        "zu",

	// Asian Languages
	"bengali":    "bn",
	"burmese":    "my",
	"cambodian":  "km",
	"khmer":      "km",
	"gujarati":   "gu",
	"indonesian": "id",
	"kannada":    "kn",
	"lao":        "lo",
	"malayalam":  "ml",
	"malay":      "ms",
	"marathi":    "mr",
	"mongolian":  "mn",
	"nepali":     "ne",
	"odia":       "or",
	"oriya":      "or",
	"punjabi":    "pa",
	"sinhala":    "si",
	"tamil":      "ta",
	"telugu":     "te",
	"thai":       "th",
	"tibetan":    "bo",
	"urdu":       "ur",
	"vietnamese": "vi",

	// Pacific Languages
	"filipino": "fil",
	"tagalog":  "tl",
	"fijian":   "fj",
	"hawaiian": "haw",
	"maori":    "mi",
	"samoan":   "sm",
	"tongan":   "to",

	// American Languages
	"haitian creole": "ht",
	"quechua":        "qu",

	// Constructed Languages
	"esperanto": "eo",
	"latin":     "la",

	// Additional Regional Variants
	"brazilian portuguese": "pt-br",
	"european portuguese":  "pt-pt",
	"mexican spanish":      "es-mx",
	"argentinian spanish":  "es-ar",
	"castilian spanish":    "es-es",
	"american english":     "en-us",
	"british english":      "en-gb",
	"australian english":   "en-au",
	"canadian english":     "en-ca",
	"canadian french":      "fr-ca",
	"swiss german":         "de-ch",
	"austrian german":      "de-at",

	// Common Aliases
	"deutsch":   "de",
	"francais":  "fr",
	"espanol":   "es",
	"italiano":  "it",
	"português": "pt",
	"русский":   "ru",
	"日本語":       "ja",
	"한국어":       "ko",
	"简体中文":      "chs",
	"繁體中文":      "cht",
	"العربية":   "ar",
	"हिन्दी":    "hi",
}

// GetLanguageCode returns the language code for the given language name
// Returns the language code and true if found, empty string and false otherwise
func GetLanguageCode(languageName string) (string, bool) {
	code, exists := LanguageMap[languageName]
	return code, exists
}
