package languages

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

func BCP47FromMKV(code string) string {
	c := code
	if c == "" {
		return "Undetermined"
	}
	c = normalize(c)
	switch c {
	case "en", "eng":
		return "English"
	case "es", "spa":
		return "Spanish"
	case "fr", "fre", "fra":
		return "French"
	case "de", "ger", "deu":
		return "German"
	case "pt", "por":
		return "Portuguese"
	case "zh", "zho", "chi", "cmn":
		return "Chinese"
	case "ja", "jpn":
		return "Japanese"
	case "ko", "kor":
		return "Korean"
	case "it", "ita":
		return "Italian"
	case "ru", "rus":
		return "Russian"
	case "ar", "ara":
		return "Arabic"
	case "hi", "hin":
		return "Hindi"
	case "nl", "dut", "nld":
		return "Dutch"
	case "sv", "swe":
		return "Swedish"
	case "pl", "pol":
		return "Polish"
	case "tr", "tur":
		return "Turkish"
	case "he", "heb":
		return "Hebrew"
	case "uk", "ukr":
		return "Ukrainian"
	case "cs", "cze", "ces":
		return "Czech"
	case "ro", "rum", "ron":
		return "Romanian"
	case "vi", "vie":
		return "Vietnamese"
	case "th", "tha":
		return "Thai"
	case "id", "ind":
		return "Indonesian"
	case "fa", "per", "fas":
		return "Persian"
	case "el", "gre", "ell":
		return "Greek"
	case "hu", "hun":
		return "Hungarian"
	case "fi", "fin":
		return "Finnish"
	case "no", "nor":
		return "Norwegian"
	case "da", "dan":
		return "Danish"
	case "bg", "bul":
		return "Bulgarian"
	case "sr", "srp":
		return "Serbian"
	case "hr", "hrv":
		return "Croatian"
	case "sk", "slk", "slo":
		return "Slovak"
	case "sl", "slv":
		return "Slovenian"
	case "lt", "lit":
		return "Lithuanian"
	case "lv", "lav":
		return "Latvian"
	case "et", "est":
		return "Estonian"
	case "ms", "msa", "may":
		return "Malay"
	case "bn", "ben":
		return "Bengali"
	case "ur", "urd":
		return "Urdu"
	case "ta", "tam":
		return "Tamil"
	case "te", "tel":
		return "Telugu"
	case "ml", "mal":
		return "Malayalam"
	case "mr", "mar":
		return "Marathi"
	case "ne", "nep":
		return "Nepali"
	case "km", "khm":
		return "Khmer"
	case "my", "bur", "mya":
		return "Burmese"
	case "bo", "tib":
		return "Tibetan"
	case "id-id":
		return "Indonesian"
	}
	return c
}

func normalize(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'A' && ch <= 'Z' {
			b = append(b, ch+32)
			continue
		}
		b = append(b, ch)
	}
	return string(b)
}
