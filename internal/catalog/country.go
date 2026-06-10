package catalog

import (
	"fmt"
	"strings"
)

const CityAllOption = cityAllOption

func Countries() []string {
	return allCountries()
}

func SplitCountry(value string) (string, string) {
	return splitCountry(value)
}

func DefaultCountryIndex(countries []string, code string) int {
	return defaultCountryIndex(countries, code)
}

func CountryIndex(countries []string, value string) int {
	return countryIndex(countries, value)
}

func StringIndex(values []string, value string) int {
	return stringIndex(values, value)
}

func FilterCountries(countries []string, query string) []string {
	return filterCountries(countries, query)
}

func CityOptions(countryCode string) []string {
	return cityOptionsForCountry(countryCode)
}

func splitCountry(value string) (string, string) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, ")") {
		start := strings.LastIndex(value, "(")
		if start >= 0 && start < len(value)-1 {
			code := strings.ToUpper(strings.TrimSpace(value[start+1 : len(value)-1]))
			name := strings.TrimSpace(value[:start])
			fields := strings.Fields(name)
			if len(fields) > 1 && len([]rune(fields[0])) <= 2 {
				name = strings.Join(fields[1:], " ")
			}
			return code, name
		}
	}
	parts := strings.SplitN(value, " - ", 2)
	if len(parts) != 2 {
		return value, value
	}
	return parts[0], parts[1]
}

func defaultCountryIndex(countries []string, code string) int {
	code = strings.ToUpper(strings.TrimSpace(code))
	for i, country := range countries {
		countryCode, _ := splitCountry(country)
		if strings.EqualFold(countryCode, code) {
			return i
		}
	}
	return 0
}

func countryIndex(countries []string, value string) int {
	for i, country := range countries {
		if strings.EqualFold(country, value) {
			return i
		}
	}
	return -1
}

func stringIndex(values []string, value string) int {
	for i, item := range values {
		if strings.EqualFold(item, value) {
			return i
		}
	}
	return -1
}

func filterCountries(countries []string, query string) []string {
	rawQuery := strings.TrimSpace(query)
	query = strings.ToLower(rawQuery)
	if query == "" {
		return append([]string{}, countries...)
	}
	if len([]rune(rawQuery)) == 2 && isASCIIAlpha(rawQuery) {
		codeQuery := strings.ToUpper(rawQuery)
		for _, country := range countries {
			code, _ := splitCountry(country)
			if strings.EqualFold(code, codeQuery) {
				return []string{country}
			}
		}
		return []string{}
	}
	filtered := []string{}
	for _, country := range countries {
		code, name := splitCountry(country)
		searchText := strings.ToLower(country + " " + code + " " + name + " " + englishCountryName(code))
		if strings.Contains(searchText, query) {
			filtered = append(filtered, country)
		}
	}
	return filtered
}

func isASCIIAlpha(value string) bool {
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return value != ""
}

func localizeCountryOptions(raw []string) []string {
	options := make([]string, 0, len(raw))
	for _, item := range raw {
		code, englishName := splitCountry(item)
		code = strings.ToUpper(strings.TrimSpace(code))
		name := chineseCountryName(code)
		if name == "" {
			name = englishName
		}
		options = append(options, fmt.Sprintf("%s (%s)", name, code))
	}
	return options
}

func chineseCountryName(code string) string {
	return chineseCountryNames()[strings.ToUpper(strings.TrimSpace(code))]
}

func englishCountryName(code string) string {
	return englishCountryNames()[strings.ToUpper(strings.TrimSpace(code))]
}

func chineseCountryNames() map[string]string {
	return map[string]string{
		"AD": "安道尔", "AE": "阿联酋", "AF": "阿富汗", "AG": "安提瓜和巴布达", "AI": "安圭拉", "AL": "阿尔巴尼亚", "AM": "亚美尼亚", "AO": "安哥拉", "AR": "阿根廷", "AS": "美属萨摩亚", "AT": "奥地利", "AU": "澳大利亚", "AW": "阿鲁巴", "AX": "奥兰群岛", "AZ": "阿塞拜疆",
		"BA": "波黑", "BB": "巴巴多斯", "BD": "孟加拉国", "BE": "比利时", "BF": "布基纳法索", "BG": "保加利亚", "BH": "巴林", "BI": "布隆迪", "BJ": "贝宁", "BL": "圣巴泰勒米", "BM": "百慕大", "BN": "文莱", "BO": "玻利维亚", "BQ": "荷兰加勒比区", "BR": "巴西", "BS": "巴哈马", "BT": "不丹", "BW": "博茨瓦纳", "BY": "白俄罗斯", "BZ": "伯利兹",
		"CA": "加拿大", "CD": "刚果（金）", "CF": "中非", "CG": "刚果（布）", "CH": "瑞士", "CI": "科特迪瓦", "CK": "库克群岛", "CL": "智利", "CM": "喀麦隆", "CN": "中国", "CO": "哥伦比亚", "CR": "哥斯达黎加", "CU": "古巴", "CV": "佛得角", "CW": "库拉索", "CY": "塞浦路斯", "CZ": "捷克",
		"DE": "德国", "DJ": "吉布提", "DK": "丹麦", "DM": "多米尼克", "DO": "多米尼加", "DZ": "阿尔及利亚",
		"EC": "厄瓜多尔", "EE": "爱沙尼亚", "EG": "埃及", "ER": "厄立特里亚", "ES": "西班牙", "ET": "埃塞俄比亚",
		"FI": "芬兰", "FJ": "斐济", "FK": "福克兰群岛", "FM": "密克罗尼西亚", "FO": "法罗群岛", "FR": "法国",
		"GA": "加蓬", "GB": "英国", "GD": "格林纳达", "GE": "格鲁吉亚", "GF": "法属圭亚那", "GG": "根西岛", "GH": "加纳", "GI": "直布罗陀", "GL": "格陵兰", "GM": "冈比亚", "GN": "几内亚", "GP": "瓜德罗普", "GQ": "赤道几内亚", "GR": "希腊", "GT": "危地马拉", "GU": "关岛", "GW": "几内亚比绍", "GY": "圭亚那",
		"HK": "中国香港", "HN": "洪都拉斯", "HR": "克罗地亚", "HT": "海地", "HU": "匈牙利",
		"ID": "印度尼西亚", "IE": "爱尔兰", "IL": "以色列", "IM": "马恩岛", "IN": "印度", "IO": "英属印度洋领地", "IQ": "伊拉克", "IR": "伊朗", "IS": "冰岛", "IT": "意大利",
		"JE": "泽西岛", "JM": "牙买加", "JO": "约旦", "JP": "日本",
		"KE": "肯尼亚", "KG": "吉尔吉斯斯坦", "KH": "柬埔寨", "KI": "基里巴斯", "KM": "科摩罗", "KN": "圣基茨和尼维斯", "KP": "朝鲜", "KR": "韩国", "KW": "科威特", "KY": "开曼群岛", "KZ": "哈萨克斯坦",
		"LA": "老挝", "LB": "黎巴嫩", "LC": "圣卢西亚", "LI": "列支敦士登", "LK": "斯里兰卡", "LR": "利比里亚", "LS": "莱索托", "LT": "立陶宛", "LU": "卢森堡", "LV": "拉脱维亚", "LY": "利比亚",
		"MA": "摩洛哥", "MC": "摩纳哥", "MD": "摩尔多瓦", "ME": "黑山", "MF": "法属圣马丁", "MG": "马达加斯加", "MH": "马绍尔群岛", "MK": "北马其顿", "ML": "马里", "MM": "缅甸", "MN": "蒙古", "MO": "中国澳门", "MP": "北马里亚纳群岛", "MQ": "马提尼克", "MR": "毛里塔尼亚", "MS": "蒙特塞拉特", "MT": "马耳他", "MU": "毛里求斯", "MV": "马尔代夫", "MW": "马拉维", "MX": "墨西哥", "MY": "马来西亚", "MZ": "莫桑比克",
		"NA": "纳米比亚", "NC": "新喀里多尼亚", "NE": "尼日尔", "NF": "诺福克岛", "NG": "尼日利亚", "NI": "尼加拉瓜", "NL": "荷兰", "NO": "挪威", "NP": "尼泊尔", "NR": "瑙鲁", "NU": "纽埃", "NZ": "新西兰",
		"OM": "阿曼", "PA": "巴拿马", "PE": "秘鲁", "PF": "法属波利尼西亚", "PG": "巴布亚新几内亚", "PH": "菲律宾", "PK": "巴基斯坦", "PL": "波兰", "PM": "圣皮埃尔和密克隆", "PN": "皮特凯恩群岛", "PR": "波多黎各", "PS": "巴勒斯坦", "PT": "葡萄牙", "PW": "帕劳", "PY": "巴拉圭",
		"QA": "卡塔尔", "RE": "留尼汪", "RO": "罗马尼亚", "RS": "塞尔维亚", "RU": "俄罗斯", "RW": "卢旺达",
		"SA": "沙特阿拉伯", "SB": "所罗门群岛", "SC": "塞舌尔", "SD": "苏丹", "SE": "瑞典", "SG": "新加坡", "SH": "圣赫勒拿", "SI": "斯洛文尼亚", "SK": "斯洛伐克", "SL": "塞拉利昂", "SM": "圣马力诺", "SN": "塞内加尔", "SO": "索马里", "SR": "苏里南", "SS": "南苏丹", "ST": "圣多美和普林西比", "SV": "萨尔瓦多", "SX": "荷属圣马丁", "SY": "叙利亚", "SZ": "斯威士兰",
		"TC": "特克斯和凯科斯群岛", "TD": "乍得", "TG": "多哥", "TH": "泰国", "TJ": "塔吉克斯坦", "TK": "托克劳", "TL": "东帝汶", "TM": "土库曼斯坦", "TN": "突尼斯", "TO": "汤加", "TR": "土耳其", "TT": "特立尼达和多巴哥", "TV": "图瓦卢", "TW": "中国台湾", "TZ": "坦桑尼亚",
		"UA": "乌克兰", "UG": "乌干达", "US": "美国", "UY": "乌拉圭", "UZ": "乌兹别克斯坦", "VA": "梵蒂冈", "VC": "圣文森特和格林纳丁斯", "VE": "委内瑞拉", "VG": "英属维尔京群岛", "VI": "美属维尔京群岛", "VN": "越南", "VU": "瓦努阿图",
		"WF": "瓦利斯和富图纳", "WS": "萨摩亚", "YE": "也门", "YT": "马约特", "ZA": "南非", "ZM": "赞比亚", "ZW": "津巴布韦",
	}
}

func englishCountryNames() map[string]string {
	names := map[string]string{}
	for _, item := range rawCountryOptions() {
		code, name := splitCountry(item)
		names[strings.ToUpper(code)] = name
	}
	return names
}

const cityAllOption = "全部城市"

func cityOptionsForCountry(countryCode string) []string {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	cities := countryCityOptions()[countryCode]
	options := []string{cityAllOption}
	options = append(options, cities...)
	return options
}

func countryCityOptions() map[string][]string {
	return map[string][]string{
		"AR": {"Buenos Aires", "Cordoba", "Mendoza", "Rosario", "Santa Fe", "Tucuman"},
		"AU": {"Australian Capital Territory", "New South Wales", "Northern Territory", "Queensland", "South Australia", "Tasmania", "Victoria", "Western Australia", "Sydney", "Melbourne", "Brisbane", "Perth"},
		"BR": {"Bahia", "Brasilia", "Ceara", "Minas Gerais", "Parana", "Pernambuco", "Rio de Janeiro", "Rio Grande do Sul", "Sao Paulo"},
		"CA": {"Alberta", "British Columbia", "Manitoba", "New Brunswick", "Newfoundland and Labrador", "Nova Scotia", "Ontario", "Prince Edward Island", "Quebec", "Saskatchewan", "Toronto", "Vancouver", "Montreal"},
		"CN": {"Anhui", "Beijing", "Chongqing", "Fujian", "Guangdong", "Guangxi", "Hebei", "Henan", "Hubei", "Hunan", "Jiangsu", "Jiangxi", "Liaoning", "Shaanxi", "Shandong", "Shanghai", "Sichuan", "Tianjin", "Zhejiang"},
		"DE": {"Baden-Wurttemberg", "Bavaria", "Berlin", "Brandenburg", "Bremen", "Hamburg", "Hesse", "Lower Saxony", "North Rhine-Westphalia", "Rhineland-Palatinate", "Saxony", "Schleswig-Holstein"},
		"ES": {"Andalusia", "Aragon", "Barcelona", "Basque Country", "Canary Islands", "Castile and Leon", "Catalonia", "Madrid", "Valencia"},
		"FR": {"Auvergne-Rhone-Alpes", "Bordeaux", "Brittany", "Grand Est", "Hauts-de-France", "Ile-de-France", "Lyon", "Marseille", "Nouvelle-Aquitaine", "Occitanie", "Paris", "Provence-Alpes-Cote d'Azur"},
		"GB": {"Birmingham", "England", "Glasgow", "Leeds", "Liverpool", "London", "Manchester", "Northern Ireland", "Scotland", "Wales"},
		"HK": {"Central and Western", "Eastern", "Kowloon", "Kwai Tsing", "New Territories", "Sha Tin", "Wan Chai", "Yuen Long"},
		"ID": {"Bali", "Bandung", "Banten", "Central Java", "East Java", "Jakarta", "Java", "Medan", "Surabaya", "West Java"},
		"IN": {"Andhra Pradesh", "Bangalore", "Chennai", "Delhi", "Gujarat", "Hyderabad", "Karnataka", "Kerala", "Kolkata", "Maharashtra", "Mumbai", "Punjab", "Rajasthan", "Tamil Nadu", "Telangana", "Uttar Pradesh", "West Bengal"},
		"IT": {"Campania", "Emilia-Romagna", "Florence", "Lazio", "Lombardy", "Milan", "Naples", "Piedmont", "Rome", "Sicily", "Tuscany", "Veneto"},
		"JP": {"Aichi", "Chiba", "Fukuoka", "Hiroshima", "Hokkaido", "Hyogo", "Kanagawa", "Kyoto", "Miyagi", "Okinawa", "Osaka", "Saitama", "Shizuoka", "Tokyo"},
		"KR": {"Busan", "Daegu", "Daejeon", "Gangwon", "Gwangju", "Gyeonggi", "Incheon", "Jeju", "Seoul", "Ulsan"},
		"MX": {"Baja California", "Chihuahua", "Guanajuato", "Jalisco", "Mexico City", "Nuevo Leon", "Puebla", "Queretaro", "Veracruz", "Yucatan"},
		"MY": {"Johor", "Kedah", "Kuala Lumpur", "Malacca", "Negeri Sembilan", "Penang", "Perak", "Sabah", "Sarawak", "Selangor"},
		"NG": {"Abia", "Abuja", "Akwa Ibom", "Anambra", "Bauchi", "Delta", "Edo", "Enugu", "Ibadan", "Kaduna", "Kano", "Lagos", "Ogun", "Ondo", "Osun", "Oyo", "Port Harcourt", "Rivers"},
		"NL": {"Amsterdam", "Eindhoven", "Gelderland", "Groningen", "North Brabant", "North Holland", "Rotterdam", "South Holland", "The Hague", "Utrecht"},
		"PH": {"Cebu", "Davao", "Makati", "Manila", "Metro Manila", "Pasig", "Quezon City", "Taguig"},
		"RU": {"Krasnodar", "Moscow", "Moscow Oblast", "Novosibirsk", "Saint Petersburg", "Samara", "Sverdlovsk", "Tatarstan", "Yekaterinburg"},
		"SG": {"Central", "East", "North", "North-East", "Queenstown", "Tampines", "Toa Payoh", "West"},
		"TH": {"Bangkok", "Chiang Mai", "Chon Buri", "Khon Kaen", "Nonthaburi", "Pathum Thani", "Phuket", "Samut Prakan"},
		"TR": {"Adana", "Ankara", "Antalya", "Bursa", "Istanbul", "Izmir", "Konya", "Mersin"},
		"TW": {"Hsinchu", "Kaohsiung", "Keelung", "New Taipei", "Taichung", "Tainan", "Taipei", "Taoyuan"},
		"US": {"Alabama", "Arizona", "California", "Colorado", "Florida", "Georgia", "Illinois", "Massachusetts", "Michigan", "Nevada", "New Jersey", "New York", "North Carolina", "Ohio", "Oregon", "Pennsylvania", "Texas", "Virginia", "Washington", "Los Angeles", "Miami", "New York City", "San Francisco", "Seattle"},
		"VN": {"Binh Duong", "Da Nang", "Dong Nai", "Ha Noi", "Hai Phong", "Ho Chi Minh City", "Khanh Hoa"},
		"ZA": {"Cape Town", "Durban", "Eastern Cape", "Gauteng", "Johannesburg", "KwaZulu-Natal", "Pretoria", "Western Cape"},
	}
}

func allCountries() []string {
	return localizeCountryOptions(rawCountryOptions())
}

func rawCountryOptions() []string {
	return []string{
		"AF - Afghanistan",
		"AX - Aland Islands",
		"AL - Albania",
		"DZ - Algeria",
		"AS - American Samoa",
		"AD - Andorra",
		"AO - Angola",
		"AI - Anguilla",
		"AQ - Antarctica",
		"AG - Antigua and Barbuda",
		"AR - Argentina",
		"AM - Armenia",
		"AW - Aruba",
		"AU - Australia",
		"AT - Austria",
		"AZ - Azerbaijan",
		"BS - Bahamas",
		"BH - Bahrain",
		"BD - Bangladesh",
		"BB - Barbados",
		"BY - Belarus",
		"BE - Belgium",
		"BZ - Belize",
		"BJ - Benin",
		"BM - Bermuda",
		"BT - Bhutan",
		"BO - Bolivia",
		"BQ - Bonaire, Sint Eustatius and Saba",
		"BA - Bosnia and Herzegovina",
		"BW - Botswana",
		"BV - Bouvet Island",
		"BR - Brazil",
		"IO - British Indian Ocean Territory",
		"BN - Brunei Darussalam",
		"BG - Bulgaria",
		"BF - Burkina Faso",
		"BI - Burundi",
		"KH - Cambodia",
		"CM - Cameroon",
		"CA - Canada",
		"CV - Cape Verde",
		"KY - Cayman Islands",
		"CF - Central African Republic",
		"TD - Chad",
		"CL - Chile",
		"CN - China",
		"CX - Christmas Island",
		"CC - Cocos Islands",
		"CO - Colombia",
		"KM - Comoros",
		"CG - Congo",
		"CD - Congo, Democratic Republic",
		"CK - Cook Islands",
		"CR - Costa Rica",
		"CI - Cote d'Ivoire",
		"HR - Croatia",
		"CU - Cuba",
		"CW - Curacao",
		"CY - Cyprus",
		"CZ - Czech Republic",
		"DK - Denmark",
		"DJ - Djibouti",
		"DM - Dominica",
		"DO - Dominican Republic",
		"EC - Ecuador",
		"EG - Egypt",
		"SV - El Salvador",
		"GQ - Equatorial Guinea",
		"ER - Eritrea",
		"EE - Estonia",
		"SZ - Eswatini",
		"ET - Ethiopia",
		"FK - Falkland Islands",
		"FO - Faroe Islands",
		"FJ - Fiji",
		"FI - Finland",
		"FR - France",
		"GF - French Guiana",
		"PF - French Polynesia",
		"TF - French Southern Territories",
		"GA - Gabon",
		"GM - Gambia",
		"GE - Georgia",
		"DE - Germany",
		"GH - Ghana",
		"GI - Gibraltar",
		"GR - Greece",
		"GL - Greenland",
		"GD - Grenada",
		"GP - Guadeloupe",
		"GU - Guam",
		"GT - Guatemala",
		"GG - Guernsey",
		"GN - Guinea",
		"GW - Guinea-Bissau",
		"GY - Guyana",
		"HT - Haiti",
		"HM - Heard Island and McDonald Islands",
		"VA - Holy See",
		"HN - Honduras",
		"HK - Hong Kong",
		"HU - Hungary",
		"IS - Iceland",
		"IN - India",
		"ID - Indonesia",
		"IR - Iran",
		"IQ - Iraq",
		"IE - Ireland",
		"IM - Isle of Man",
		"IL - Israel",
		"IT - Italy",
		"JM - Jamaica",
		"JP - Japan",
		"JE - Jersey",
		"JO - Jordan",
		"KZ - Kazakhstan",
		"KE - Kenya",
		"KI - Kiribati",
		"KP - Korea, Democratic People's Republic",
		"KR - Korea, Republic",
		"KW - Kuwait",
		"KG - Kyrgyzstan",
		"LA - Lao People's Democratic Republic",
		"LV - Latvia",
		"LB - Lebanon",
		"LS - Lesotho",
		"LR - Liberia",
		"LY - Libya",
		"LI - Liechtenstein",
		"LT - Lithuania",
		"LU - Luxembourg",
		"MO - Macao",
		"MG - Madagascar",
		"MW - Malawi",
		"MY - Malaysia",
		"MV - Maldives",
		"ML - Mali",
		"MT - Malta",
		"MH - Marshall Islands",
		"MQ - Martinique",
		"MR - Mauritania",
		"MU - Mauritius",
		"YT - Mayotte",
		"MX - Mexico",
		"FM - Micronesia",
		"MD - Moldova",
		"MC - Monaco",
		"MN - Mongolia",
		"ME - Montenegro",
		"MS - Montserrat",
		"MA - Morocco",
		"MZ - Mozambique",
		"MM - Myanmar",
		"NA - Namibia",
		"NR - Nauru",
		"NP - Nepal",
		"NL - Netherlands",
		"NC - New Caledonia",
		"NZ - New Zealand",
		"NI - Nicaragua",
		"NE - Niger",
		"NG - Nigeria",
		"NU - Niue",
		"NF - Norfolk Island",
		"MK - North Macedonia",
		"MP - Northern Mariana Islands",
		"NO - Norway",
		"OM - Oman",
		"PK - Pakistan",
		"PW - Palau",
		"PS - Palestine",
		"PA - Panama",
		"PG - Papua New Guinea",
		"PY - Paraguay",
		"PE - Peru",
		"PH - Philippines",
		"PN - Pitcairn",
		"PL - Poland",
		"PT - Portugal",
		"PR - Puerto Rico",
		"QA - Qatar",
		"RE - Reunion",
		"RO - Romania",
		"RU - Russian Federation",
		"RW - Rwanda",
		"BL - Saint Barthelemy",
		"SH - Saint Helena",
		"KN - Saint Kitts and Nevis",
		"LC - Saint Lucia",
		"MF - Saint Martin",
		"PM - Saint Pierre and Miquelon",
		"VC - Saint Vincent and the Grenadines",
		"WS - Samoa",
		"SM - San Marino",
		"ST - Sao Tome and Principe",
		"SA - Saudi Arabia",
		"SN - Senegal",
		"RS - Serbia",
		"SC - Seychelles",
		"SL - Sierra Leone",
		"SG - Singapore",
		"SX - Sint Maarten",
		"SK - Slovakia",
		"SI - Slovenia",
		"SB - Solomon Islands",
		"SO - Somalia",
		"ZA - South Africa",
		"GS - South Georgia and the South Sandwich Islands",
		"SS - South Sudan",
		"ES - Spain",
		"LK - Sri Lanka",
		"SD - Sudan",
		"SR - Suriname",
		"SJ - Svalbard and Jan Mayen",
		"SE - Sweden",
		"CH - Switzerland",
		"SY - Syrian Arab Republic",
		"TW - Taiwan",
		"TJ - Tajikistan",
		"TZ - Tanzania",
		"TH - Thailand",
		"TL - Timor-Leste",
		"TG - Togo",
		"TK - Tokelau",
		"TO - Tonga",
		"TT - Trinidad and Tobago",
		"TN - Tunisia",
		"TR - Turkiye",
		"TM - Turkmenistan",
		"TC - Turks and Caicos Islands",
		"TV - Tuvalu",
		"UG - Uganda",
		"UA - Ukraine",
		"AE - United Arab Emirates",
		"GB - United Kingdom",
		"US - United States",
		"UM - United States Minor Outlying Islands",
		"UY - Uruguay",
		"UZ - Uzbekistan",
		"VU - Vanuatu",
		"VE - Venezuela",
		"VN - Viet Nam",
		"VG - Virgin Islands, British",
		"VI - Virgin Islands, U.S.",
		"WF - Wallis and Futuna",
		"EH - Western Sahara",
		"YE - Yemen",
		"ZM - Zambia",
		"ZW - Zimbabwe",
	}
}
