package main

import (
	"html"
	"net/http"
	"strings"
)

type langInfo struct {
	Code, Name string
}

var languages = []langInfo{
	{"en", "English"},
	{"ru", "Русский"},
	{"zh", "中文"},
	{"es", "Español"},
	{"de", "Deutsch"},
	{"fr", "Français"},
	{"ja", "日本語"},
	{"pt", "Português"},
}

var catalog = map[string]map[string]string{
	"en": {
		"nav.live": "Live", "nav.stats": "Device", "nav.logs": "Logs",
		"nav.setup": "Settings", "nav.logout": "Log out", "lang": "Language",
		"login.title": "Sign in", "login.password": "Password", "login.submit": "Sign in",
		"login.wrong": "Wrong password.", "login.ratelimit": "Too many attempts. Wait 10 minutes.",
		"init.title": "Set site password", "init.hint": "This is the page password, not your EZVIZ account.",
		"init.password": "Site password", "init.save": "Save", "init.min8": "At least 8 characters.",
		"error":       "Error",
		"setup.title": "Settings", "setup.ezviz": "EZVIZ account", "setup.email": "EZVIZ account email", "setup.ezvizpw": "EZVIZ password",
		"setup.region": "Region", "setup.save": "Save and start",
		"setup.devicesHint": "Cameras are listed on Live — switch there.",
		"setup.saved":       "Saved. The camera is waking — video in about 20 seconds.",
		"setup.sitepw":      "Site login password",
		"setup.current":     "Current password", "setup.new": "New password", "setup.repeat": "Again",
		"setup.change": "Change password", "setup.pwBad": "Current site password is wrong.", "setup.pwShort": "New password must be at least 8 characters.",
		"setup.pwMismatch": "New password and confirmation do not match.", "setup.pwOk": "Site password changed.",
		"setup.stream": "Stream", "setup.streamOnDemand": "On demand", "setup.streamAlways": "Always on",
		"setup.streamHint":          "On demand: streams only while Live is open, sleeps ~10s idle. Always on: never stops, for battery tests.",
		"setup.streamSavedOnDemand": "On demand. Sleeps ~10s after nobody is watching.",
		"setup.streamSavedAlways":   "Always on. Camera stays awake; watch Battery.",
		"setup.update":              "Updates",
		"setup.updateCheck":         "Check for updates", "setup.upToDate": "Already up to date.",
		"setup.updated": "Updated.", "setup.updateFail": "Update failed.",
		"setup.updating": "Checking…", "setup.downloading": "Downloading…", "setup.installing": "Installing…", "setup.restarting": "Restarting…",
		"live.title": "Live", "live.save": "Save",
		"live.noRtc":   "This browser cannot play HEVC video.",
		"live.relogin": "Please sign in again.", "live.notConfigured": "Camera is not set up — open Settings.",
		"live.lastError": "Last error: ", "live.saving": "Saving buffer…",
		"live.downloaded": "Downloaded: ", "live.saveFail": "Could not save: ",
		"save.inactive": "stream is not active", "save.empty": "buffer is empty",
		"live.saveErr": "Save failed",
		"live.battery": "Battery", "live.online": "online", "live.offline": "offline",
		"live.upgrade":      "firmware update available",
		"live.alwaysOn":     "Always-on stream: the camera will not sleep. Turn it off in Settings.",
		"live.switchFail":   "Could not switch camera.",
		"live.waiting":      "Waiting for camera…",
		"live.reconnecting": "Reconnecting…",
		"stats.title":       "Device", "stats.day": "Day", "stats.week": "Week", "stats.month": "Month",
		"stats.points":     "Samples: %s · every %s min",
		"stats.empty":      "No battery data yet — the first sample appears within %s minutes.",
		"stats.fail":       "Could not load battery history.",
		"stats.poll":       "Background poll",
		"stats.pollCustom": "Minutes (1–1440)", "stats.pollSave": "Save",
		"stats.pollBad":   "Enter 1–1440 minutes.",
		"stats.pollSaved": "Interval saved for background history. Live/status also refresh on each visit (cloud API, does not wake the camera).",
		"stats.pollHint":  "One cloud request reads battery, Wi‑Fi, online and firmware flag. Does not wake the camera. Also refreshes when you open the site.",
		"logs.title":      "Logs", "logs.download": "download all", "logs.limit": "archive limit 100 MB",
		"logs.shown": " · %.1f KB on screen of %.1f MB", "logs.empty": "(empty)",
		"logs.lez": "Bridge (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Bridge logging", "logs.on": "On", "logs.off": "Off",
		"logs.levelHint":    "Writes verbose bridge logs (every packet, ~45 MB/day) to lez.log. Applies on the next stream start.",
		"logs.saved":        "Saved. It applies when the stream next starts.",
		"logs.clear":        "Clear log files",
		"logs.clearConfirm": "Clear lez.log, ffmpeg.log and bridge.err?",
		"maint.clearStats":  "Reset battery history",
	},
	"ru": {
		"nav.live": "Стрим", "nav.stats": "Устройство", "nav.logs": "Логи",
		"nav.setup": "Настройки", "nav.logout": "Выйти", "lang": "Язык",
		"login.title": "Вход", "login.password": "Пароль", "login.submit": "Войти",
		"login.wrong": "Неверный пароль.", "login.ratelimit": "Слишком много попыток. Подождите 10 минут.",
		"init.title": "Задайте пароль сайта", "init.hint": "Это пароль входа на страницу камеры (не EZVIZ).",
		"init.password": "Пароль сайта", "init.save": "Сохранить", "init.min8": "Минимум 8 символов.",
		"error":       "Ошибка",
		"setup.title": "Настройки", "setup.ezviz": "Аккаунт EZVIZ", "setup.email": "Email аккаунта EZVIZ", "setup.ezvizpw": "Пароль EZVIZ",
		"setup.region": "Регион", "setup.save": "Сохранить и запустить",
		"setup.devicesHint": "Камеры выбираются на главной — переключатель над видео.",
		"setup.saved":       "Сохранено. Камера просыпается — видео появится через ~20 секунд.",
		"setup.sitepw":      "Пароль входа на сайт",
		"setup.current":     "Текущий пароль", "setup.new": "Новый пароль", "setup.repeat": "Ещё раз",
		"setup.change": "Сменить пароль", "setup.pwBad": "Неверный текущий пароль сайта.", "setup.pwShort": "Новый пароль — минимум 8 символов.",
		"setup.pwMismatch": "Новый пароль и подтверждение не совпадают.", "setup.pwOk": "Пароль сайта изменён.",
		"setup.stream": "Стрим", "setup.streamOnDemand": "По требованию", "setup.streamAlways": "Постоянно",
		"setup.streamHint":          "По требованию: стрим только пока открыт Live, сон ~через 10 с. Постоянно: не гаснет, удобно мерить батарею.",
		"setup.streamSavedOnDemand": "По требованию. Без зрителей камера засыпает ~через 10 с.",
		"setup.streamSavedAlways":   "Постоянно. Камера не засыпает; смотрите Батарею.",
		"setup.update":              "Обновления",
		"setup.updateCheck":         "Проверить обновления", "setup.upToDate": "Уже актуальная версия.",
		"setup.updated": "Обновлено.", "setup.updateFail": "Обновление не удалось.",
		"setup.updating": "Проверка…", "setup.downloading": "Скачивание…", "setup.installing": "Установка…", "setup.restarting": "Перезапуск…",
		"live.title": "Стрим", "live.save": "Сохранить",
		"live.noRtc":   "Браузер не умеет HEVC-видео.",
		"live.relogin": "Нужно войти заново.", "live.notConfigured": "Камера не настроена — откройте Настройки.",
		"live.lastError": "Последняя ошибка: ", "live.saving": "Сохраняю буфер…",
		"live.downloaded": "Скачано: ", "live.saveFail": "Не сохранилось: ",
		"save.inactive": "стрим не активен", "save.empty": "буфер пуст",
		"live.saveErr": "Ошибка сохранения",
		"live.battery": "Батарея", "live.online": "онлайн", "live.offline": "офлайн",
		"live.upgrade":      "есть новая прошивка",
		"live.alwaysOn":     "Постоянный стрим: камера не засыпает. Выключить можно в Настройках.",
		"live.switchFail":   "Не удалось переключить камеру.",
		"live.waiting":      "Ожидание камеры…",
		"live.reconnecting": "Переподключение…",
		"stats.title":       "Устройство", "stats.day": "Сутки", "stats.week": "Неделя", "stats.month": "Месяц",
		"stats.points":     "Точек: %s · каждые %s мин",
		"stats.empty":      "Пока нет данных — первая точка появится в течение %s минут.",
		"stats.fail":       "Не удалось загрузить историю батареи.",
		"stats.poll":       "Фоновый опрос",
		"stats.pollCustom": "Минуты (1–1440)", "stats.pollSave": "Сохранить",
		"stats.pollBad":   "Укажите 1–1440 минут.",
		"stats.pollSaved": "Интервал сохранён для фоновой истории. Статус/Стрим также обновляются при каждом входе (облачный API, камеру не будит).",
		"stats.pollHint":  "Один запрос в облако: батарея, Wi‑Fi, онлайн и флаг прошивки. Камеру не будит. Также обновляется при входе на сайт.",
		"logs.title":      "Логи", "logs.download": "скачать целиком", "logs.limit": "лимит архива 100 МБ",
		"logs.shown": " · %.1f КБ на экране из %.1f МБ", "logs.empty": "(пусто)",
		"logs.lez": "Мост (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Логирование моста", "logs.on": "Вкл", "logs.off": "Выкл",
		"logs.levelHint":    "Пишет подробные логи моста (каждый пакет, ~45 МБ/день) в lez.log. Применяется при следующем старте стрима.",
		"logs.saved":        "Сохранено. Сработает, когда стрим запустится снова.",
		"logs.clear":        "Очистить файлы логов",
		"logs.clearConfirm": "Очистить lez.log, ffmpeg.log и bridge.err?",
		"maint.clearStats":  "Сбросить историю батареи",
	},
	"zh": {
		"nav.live": "直播", "nav.stats": "设备", "nav.logs": "日志",
		"nav.setup": "设置", "nav.logout": "退出", "lang": "语言",
		"login.title": "登录", "login.password": "密码", "login.submit": "登录",
		"login.wrong": "密码错误。", "login.ratelimit": "尝试次数过多，请等待 10 分钟。",
		"init.title": "设置网站密码", "init.hint": "这是网页密码，不是萤石账号。",
		"init.password": "网站密码", "init.save": "保存", "init.min8": "至少 8 个字符。",
		"error":       "错误",
		"setup.title": "设置", "setup.ezviz": "萤石账号", "setup.email": "萤石账号邮箱", "setup.ezvizpw": "萤石密码",
		"setup.region": "地区", "setup.save": "保存并启动",
		"setup.devicesHint": "保存后在直播页切换摄像机。",
		"setup.saved":       "已保存。摄像机正在唤醒，约 20 秒后出画面。",
		"setup.sitepw":      "网站登录密码",
		"setup.current":     "当前密码", "setup.new": "新密码", "setup.repeat": "再输入一次",
		"setup.change": "修改密码", "setup.pwBad": "当前网站密码不正确。", "setup.pwShort": "新密码至少 8 个字符。",
		"setup.pwMismatch": "两次新密码不一致。", "setup.pwOk": "网站密码已更改。",
		"setup.stream": "直播", "setup.streamOnDemand": "按需", "setup.streamAlways": "持续",
		"setup.streamHint":          "按需：仅在打开直播时推流，空闲约 10 秒后休眠。持续：不自动停，便于测电量。",
		"setup.streamSavedOnDemand": "已改为按需。无人观看约 10 秒后休眠。",
		"setup.streamSavedAlways":   "持续直播。摄像机保持唤醒，请看电量页。",
		"setup.update":              "更新",
		"setup.updateCheck":         "检查更新", "setup.upToDate": "已是最新版本。",
		"setup.updated": "已更新。", "setup.updateFail": "更新失败。",
		"setup.updating": "检查中…", "setup.downloading": "下载中…", "setup.installing": "安装中…", "setup.restarting": "重启中…",
		"live.title": "直播", "live.save": "保存",
		"live.noRtc":   "浏览器不支持 HEVC 视频。",
		"live.relogin": "请重新登录。", "live.notConfigured": "摄像机未配置 — 请打开设置。",
		"live.lastError": "上次错误：", "live.saving": "正在保存缓冲…",
		"live.downloaded": "已下载：", "live.saveFail": "保存失败：",
		"save.inactive": "直播未启动", "save.empty": "缓冲区为空",
		"live.saveErr": "保存出错",
		"live.battery": "电量", "live.online": "在线", "live.offline": "离线",
		"live.upgrade":      "有新固件",
		"live.alwaysOn":     "持续直播：摄像机不会休眠。可在设置中关闭。",
		"live.switchFail":   "无法切换摄像机。",
		"live.waiting":      "等待摄像机…",
		"live.reconnecting": "重新连接…",
		"stats.title":       "设备", "stats.day": "一天", "stats.week": "一周", "stats.month": "一月",
		"stats.points":     "采样：%s · 每 %s 分钟",
		"stats.empty":      "暂无电量数据，约 %s 分钟内出现第一个点。",
		"stats.fail":       "无法加载电量历史。",
		"stats.poll":       "后台轮询",
		"stats.pollCustom": "分钟（1–1440）", "stats.pollSave": "保存",
		"stats.pollBad":   "请输入 1–1440 分钟。",
		"stats.pollSaved": "间隔已保存，用于后台历史。每次打开站点时也会刷新直播/状态（云端 API，不会唤醒摄像机）。",
		"stats.pollHint":  "一次云端请求读取电量、Wi‑Fi、在线和固件标志。不会唤醒摄像机。打开站点时也会刷新。",
		"logs.title":      "日志", "logs.download": "下载全部", "logs.limit": "日志上限 100 MB",
		"logs.shown": " · 屏幕 %.1f KB / 共 %.1f MB", "logs.empty": "（空）",
		"logs.lez": "桥接 (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "桥接日志", "logs.on": "开", "logs.off": "关",
		"logs.levelHint":    "将详细的桥接日志（每个数据包，约 45 MB/天）写入 lez.log。下次启动直播时生效。",
		"logs.saved":        "已保存，将在下次启动直播时生效。",
		"logs.clear":        "清空日志文件",
		"logs.clearConfirm": "清空 lez.log、ffmpeg.log 和 bridge.err？",
		"maint.clearStats":  "重置电量历史",
	},
	"es": {
		"nav.live": "En vivo", "nav.stats": "Dispositivo", "nav.logs": "Registros",
		"nav.setup": "Ajustes", "nav.logout": "Salir", "lang": "Idioma",
		"login.title": "Entrar", "login.password": "Contraseña", "login.submit": "Entrar",
		"login.wrong": "Contraseña incorrecta.", "login.ratelimit": "Demasiados intentos. Espere 10 minutos.",
		"init.title": "Contraseña del sitio", "init.hint": "Es la contraseña de la página, no de EZVIZ.",
		"init.password": "Contraseña del sitio", "init.save": "Guardar", "init.min8": "Mínimo 8 caracteres.",
		"error":       "Error",
		"setup.title": "Ajustes", "setup.ezviz": "Cuenta EZVIZ", "setup.email": "Email de EZVIZ", "setup.ezvizpw": "Contraseña EZVIZ",
		"setup.region": "Región", "setup.save": "Guardar e iniciar",
		"setup.devicesHint": "Cámaras en Directo — cámbielas allí.",
		"setup.saved":       "Guardado. La cámara se despierta — vídeo en ~20 s.",
		"setup.sitepw":      "Contraseña de la página",
		"setup.current":     "Contraseña actual", "setup.new": "Nueva contraseña", "setup.repeat": "Otra vez",
		"setup.change": "Cambiar contraseña", "setup.pwBad": "Contraseña actual incorrecta.", "setup.pwShort": "Mínimo 8 caracteres.",
		"setup.pwMismatch": "Las contraseñas no coinciden.", "setup.pwOk": "Contraseña cambiada.",
		"setup.stream": "Directo", "setup.streamOnDemand": "Bajo demanda", "setup.streamAlways": "Continuo",
		"setup.streamHint":          "Bajo demanda: solo con el directo abierto, duerme ~10 s. Continuo: no para, para medir la batería.",
		"setup.streamSavedOnDemand": "Bajo demanda. Duerme ~10 s si nadie mira.",
		"setup.streamSavedAlways":   "Continuo. La cámara no duerme; mire Batería.",
		"setup.update":              "Actualizaciones",
		"setup.updateCheck":         "Comprobar actualizaciones", "setup.upToDate": "Ya está actualizado.",
		"setup.updated": "Actualizado.", "setup.updateFail": "Error al actualizar.",
		"setup.updating": "Comprobando…", "setup.downloading": "Descargando…", "setup.installing": "Instalando…", "setup.restarting": "Reiniciando…",
		"live.title": "En vivo", "live.save": "Guardar",
		"live.noRtc":   "El navegador no reproduce HEVC.",
		"live.relogin": "Vuelva a iniciar sesión.", "live.notConfigured": "Cámara no configurada — abra Ajustes.",
		"live.lastError": "Último error: ", "live.saving": "Guardando búfer…",
		"live.downloaded": "Descargado: ", "live.saveFail": "No se guardó: ",
		"save.inactive": "el directo no está activo", "save.empty": "el búfer está vacío",
		"live.saveErr": "Error al guardar",
		"live.battery": "Batería", "live.online": "en línea", "live.offline": "fuera de línea",
		"live.upgrade":      "hay firmware nuevo",
		"live.alwaysOn":     "Directo continuo: la cámara no dormirá. Apáguelo en Ajustes.",
		"live.switchFail":   "No se pudo cambiar de cámara.",
		"live.waiting":      "Esperando cámara…",
		"live.reconnecting": "Reconectando…",
		"stats.title":       "Dispositivo", "stats.day": "Día", "stats.week": "Semana", "stats.month": "Mes",
		"stats.points":     "Muestras: %s · cada %s min",
		"stats.empty":      "Aún no hay datos de batería — la primera muestra llega en %s minutos.",
		"stats.fail":       "No se pudo cargar el historial de batería.",
		"stats.poll":       "Consulta en segundo plano",
		"stats.pollCustom": "Minutos (1–1440)", "stats.pollSave": "Guardar",
		"stats.pollBad":   "Introduzca 1–1440 minutos.",
		"stats.pollSaved": "Intervalo guardado para el historial en segundo plano. En vivo/estado también se actualizan en cada visita (API en la nube, no despierta la cámara).",
		"stats.pollHint":  "Una petición a la nube lee batería, Wi‑Fi, en línea y el indicador de firmware. No despierta la cámara. También se actualiza al abrir el sitio.",
		"logs.title":      "Registros", "logs.download": "descargar todo", "logs.limit": "límite 100 MB",
		"logs.shown": " · %.1f KB en pantalla de %.1f MB", "logs.empty": "(vacío)",
		"logs.lez": "Puente (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Registro del puente", "logs.on": "Activado", "logs.off": "Desactivado",
		"logs.levelHint":    "Escribe registros detallados del puente (cada paquete, ~45 MB/día) en lez.log. Se aplica al siguiente arranque del directo.",
		"logs.saved":        "Guardado. Se aplica cuando el directo vuelva a arrancar.",
		"logs.clear":        "Vaciar archivos de registro",
		"logs.clearConfirm": "¿Vaciar lez.log, ffmpeg.log y bridge.err?",
		"maint.clearStats":  "Reiniciar historial de batería",
	},
	"de": {
		"nav.live": "Live", "nav.stats": "Gerät", "nav.logs": "Logs",
		"nav.setup": "Einstellungen", "nav.logout": "Abmelden", "lang": "Sprache",
		"login.title": "Anmeldung", "login.password": "Passwort", "login.submit": "Anmelden",
		"login.wrong": "Falsches Passwort.", "login.ratelimit": "Zu viele Versuche. 10 Minuten warten.",
		"init.title": "Seitenpasswort setzen", "init.hint": "Das ist das Seitenpasswort, nicht EZVIZ.",
		"init.password": "Seitenpasswort", "init.save": "Speichern", "init.min8": "Mindestens 8 Zeichen.",
		"error":       "Fehler",
		"setup.title": "Einstellungen", "setup.ezviz": "EZVIZ-Konto", "setup.email": "EZVIZ-E-Mail", "setup.ezvizpw": "EZVIZ-Passwort",
		"setup.region": "Region", "setup.save": "Speichern und starten",
		"setup.devicesHint": "Kameras auf Live umschalten.",
		"setup.saved":       "Gespeichert. Kamera wacht auf — Bild in ~20 Sekunden.",
		"setup.sitepw":      "Login-Passwort der Seite",
		"setup.current":     "Aktuelles Passwort", "setup.new": "Neues Passwort", "setup.repeat": "Wiederholen",
		"setup.change": "Passwort ändern", "setup.pwBad": "Aktuelles Passwort ist falsch.", "setup.pwShort": "Mindestens 8 Zeichen.",
		"setup.pwMismatch": "Passwörter stimmen nicht überein.", "setup.pwOk": "Passwort geändert.",
		"setup.stream": "Stream", "setup.streamOnDemand": "Bei Bedarf", "setup.streamAlways": "Dauerhaft",
		"setup.streamHint":          "Bei Bedarf: nur bei offenem Live, Schlaf ~10 s. Dauerhaft: stoppt nicht, zum Akkutest.",
		"setup.streamSavedOnDemand": "Bei Bedarf. Schläft ~10 s ohne Zuschauer.",
		"setup.streamSavedAlways":   "Dauerhaft. Die Kamera bleibt wach; siehe Akku.",
		"setup.update":              "Updates",
		"setup.updateCheck":         "Auf Updates prüfen", "setup.upToDate": "Bereits aktuell.",
		"setup.updated": "Aktualisiert.", "setup.updateFail": "Update fehlgeschlagen.",
		"setup.updating": "Prüfen…", "setup.downloading": "Herunterladen…", "setup.installing": "Installieren…", "setup.restarting": "Neustart…",
		"live.title": "Live", "live.save": "Speichern",
		"live.noRtc":   "Browser kann HEVC nicht abspielen.",
		"live.relogin": "Bitte neu anmelden.", "live.notConfigured": "Kamera nicht eingerichtet — Einstellungen öffnen.",
		"live.lastError": "Letzter Fehler: ", "live.saving": "Puffer wird gespeichert…",
		"live.downloaded": "Heruntergeladen: ", "live.saveFail": "Nicht gespeichert: ",
		"save.inactive": "Stream ist nicht aktiv", "save.empty": "Puffer ist leer",
		"live.saveErr": "Speichern fehlgeschlagen",
		"live.battery": "Akku", "live.online": "online", "live.offline": "offline",
		"live.upgrade":      "neue Firmware",
		"live.alwaysOn":     "Dauerstream: die Kamera schläft nicht. In den Einstellungen aus.",
		"live.switchFail":   "Kamera konnte nicht gewechselt werden.",
		"live.waiting":      "Warte auf Kamera…",
		"live.reconnecting": "Verbindung wird wiederhergestellt…",
		"stats.title":       "Gerät", "stats.day": "Tag", "stats.week": "Woche", "stats.month": "Monat",
		"stats.points":     "Messwerte: %s · alle %s Min.",
		"stats.empty":      "Noch keine Akkudaten — erster Punkt innerhalb von %s Minuten.",
		"stats.fail":       "Akkuhistorie konnte nicht geladen werden.",
		"stats.poll":       "Hintergrundabfrage",
		"stats.pollCustom": "Minuten (1–1440)", "stats.pollSave": "Speichern",
		"stats.pollBad":   "1–1440 Minuten eingeben.",
		"stats.pollSaved": "Intervall für die Hintergrundhistorie gespeichert. Live/Status werden bei jedem Besuch auch aktualisiert (Cloud-API, weckt die Kamera nicht).",
		"stats.pollHint":  "Eine Cloud-Anfrage liest Akku, Wi‑Fi, Online und Firmware-Flag. Weckt die Kamera nicht. Aktualisiert auch beim Öffnen der Seite.",
		"logs.title":      "Logs", "logs.download": "alles laden", "logs.limit": "Limit 100 MB",
		"logs.shown": " · %.1f KB auf dem Schirm von %.1f MB", "logs.empty": "(leer)",
		"logs.lez": "Bridge (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Bridge-Logging", "logs.on": "Ein", "logs.off": "Aus",
		"logs.levelHint":    "Schreibt ausführliche Bridge-Logs (jedes Paket, ~45 MB/Tag) in lez.log. Gilt beim nächsten Stream-Start.",
		"logs.saved":        "Gespeichert. Gilt, wenn der Stream als Nächstes startet.",
		"logs.clear":        "Logdateien leeren",
		"logs.clearConfirm": "lez.log, ffmpeg.log und bridge.err leeren?",
		"maint.clearStats":  "Akkuhistorie zurücksetzen",
	},
	"fr": {
		"nav.live": "Direct", "nav.stats": "Appareil", "nav.logs": "Journaux",
		"nav.setup": "Réglages", "nav.logout": "Quitter", "lang": "Langue",
		"login.title": "Connexion", "login.password": "Mot de passe", "login.submit": "Connexion",
		"login.wrong": "Mot de passe incorrect.", "login.ratelimit": "Trop de tentatives. Attendez 10 minutes.",
		"init.title": "Mot de passe du site", "init.hint": "Mot de passe de la page, pas du compte EZVIZ.",
		"init.password": "Mot de passe du site", "init.save": "Enregistrer", "init.min8": "8 caractères minimum.",
		"error":       "Erreur",
		"setup.title": "Réglages", "setup.ezviz": "Compte EZVIZ", "setup.email": "E-mail EZVIZ", "setup.ezvizpw": "Mot de passe EZVIZ",
		"setup.region": "Région", "setup.save": "Enregistrer et démarrer",
		"setup.devicesHint": "Caméras sur Direct — changez-les là.",
		"setup.saved":       "Enregistré. La caméra se réveille — image dans ~20 s.",
		"setup.sitepw":      "Mot de passe du site",
		"setup.current":     "Mot de passe actuel", "setup.new": "Nouveau mot de passe", "setup.repeat": "Encore une fois",
		"setup.change": "Changer le mot de passe", "setup.pwBad": "Mot de passe actuel incorrect.", "setup.pwShort": "8 caractères minimum.",
		"setup.pwMismatch": "Les mots de passe ne correspondent pas.", "setup.pwOk": "Mot de passe modifié.",
		"setup.stream": "Direct", "setup.streamOnDemand": "À la demande", "setup.streamAlways": "En continu",
		"setup.streamHint":          "À la demande : seulement pendant le Direct, veille ~10 s. En continu : ne s’arrête pas, pour la batterie.",
		"setup.streamSavedOnDemand": "À la demande. Veille ~10 s sans spectateur.",
		"setup.streamSavedAlways":   "En continu. La caméra reste éveillée ; voir Batterie.",
		"setup.update":              "Mises à jour",
		"setup.updateCheck":         "Vérifier les mises à jour", "setup.upToDate": "Déjà à jour.",
		"setup.updated": "Mis à jour.", "setup.updateFail": "Échec de la mise à jour.",
		"setup.updating": "Vérification…", "setup.downloading": "Téléchargement…", "setup.installing": "Installation…", "setup.restarting": "Redémarrage…",
		"live.title": "Direct", "live.save": "Enregistrer",
		"live.noRtc":   "Ce navigateur ne lit pas HEVC.",
		"live.relogin": "Reconnectez-vous.", "live.notConfigured": "Caméra non configurée — ouvrez Réglages.",
		"live.lastError": "Dernière erreur : ", "live.saving": "Enregistrement du tampon…",
		"live.downloaded": "Téléchargé : ", "live.saveFail": "Échec : ",
		"save.inactive": "le direct n'est pas actif", "save.empty": "le tampon est vide",
		"live.saveErr": "Erreur d’enregistrement",
		"live.battery": "Batterie", "live.online": "en ligne", "live.offline": "hors ligne",
		"live.upgrade":      "nouveau firmware",
		"live.alwaysOn":     "Direct continu : la caméra ne dort pas. Désactivez dans Réglages.",
		"live.switchFail":   "Impossible de changer de caméra.",
		"live.waiting":      "En attente de la caméra…",
		"live.reconnecting": "Reconnexion…",
		"stats.title":       "Appareil", "stats.day": "Jour", "stats.week": "Semaine", "stats.month": "Mois",
		"stats.points":     "Échantillons : %s · toutes les %s min",
		"stats.empty":      "Pas encore de données batterie — premier point dans %s minutes.",
		"stats.fail":       "Impossible de charger l’historique batterie.",
		"stats.poll":       "Interrogation en arrière-plan",
		"stats.pollCustom": "Minutes (1–1440)", "stats.pollSave": "Enregistrer",
		"stats.pollBad":   "Entrez 1–1440 minutes.",
		"stats.pollSaved": "Intervalle enregistré pour l’historique en arrière-plan. Direct/état se rafraîchissent aussi à chaque visite (API cloud, ne réveille pas la caméra).",
		"stats.pollHint":  "Une requête cloud lit batterie, Wi‑Fi, en ligne et l’indicateur firmware. Ne réveille pas la caméra. Se rafraîchit aussi à l’ouverture du site.",
		"logs.title":      "Journaux", "logs.download": "tout télécharger", "logs.limit": "limite 100 Mo",
		"logs.shown": " · %.1f Ko à l’écran sur %.1f Mo", "logs.empty": "(vide)",
		"logs.lez": "Pont (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Journaux du pont", "logs.on": "Activé", "logs.off": "Désactivé",
		"logs.levelHint":    "Écrit des journaux détaillés du pont (chaque paquet, ~45 Mo/jour) dans lez.log. Prend effet au prochain démarrage du direct.",
		"logs.saved":        "Enregistré. S’appliquera au prochain démarrage du direct.",
		"logs.clear":        "Vider les fichiers journaux",
		"logs.clearConfirm": "Vider lez.log, ffmpeg.log et bridge.err ?",
		"maint.clearStats":  "Réinit. historique batterie",
	},
	"ja": {
		"nav.live": "ライブ", "nav.stats": "デバイス", "nav.logs": "ログ",
		"nav.setup": "設定", "nav.logout": "ログアウト", "lang": "言語",
		"login.title": "ログイン", "login.password": "パスワード", "login.submit": "ログイン",
		"login.wrong": "パスワードが違います。", "login.ratelimit": "試行が多すぎます。10分待ってください。",
		"init.title": "サイトのパスワード", "init.hint": "EZVIZアカウントではなく、このページのパスワードです。",
		"init.password": "サイトのパスワード", "init.save": "保存", "init.min8": "8文字以上。",
		"error":       "エラー",
		"setup.title": "設定", "setup.ezviz": "EZVIZアカウント", "setup.email": "EZVIZメール", "setup.ezvizpw": "EZVIZパスワード",
		"setup.region": "地域", "setup.save": "保存して開始",
		"setup.devicesHint": "ライブ画面でカメラを切り替えます。",
		"setup.saved":       "保存しました。カメラ起動中 — 約20秒で映像が出ます。",
		"setup.sitepw":      "サイトのログインパスワード",
		"setup.current":     "現在のパスワード", "setup.new": "新しいパスワード", "setup.repeat": "再入力",
		"setup.change": "パスワードを変更", "setup.pwBad": "現在のパスワードが違います。", "setup.pwShort": "8文字以上にしてください。",
		"setup.pwMismatch": "パスワードが一致しません。", "setup.pwOk": "パスワードを変更しました。",
		"setup.stream": "配信", "setup.streamOnDemand": "オンデマンド", "setup.streamAlways": "常時",
		"setup.streamHint":          "オンデマンド：ライブ表示中のみ、約10秒でスリープ。常時：止まらないのでバッテリー計測向け。",
		"setup.streamSavedOnDemand": "オンデマンド。視聴者がいなくなると約10秒でスリープ。",
		"setup.streamSavedAlways":   "常時。カメラは起き続けます。バッテリーを見てください。",
		"setup.update":              "更新",
		"setup.updateCheck":         "更新を確認", "setup.upToDate": "最新です。",
		"setup.updated": "更新しました。", "setup.updateFail": "更新に失敗しました。",
		"setup.updating": "確認中…", "setup.downloading": "ダウンロード中…", "setup.installing": "インストール中…", "setup.restarting": "再起動中…",
		"live.title": "ライブ", "live.save": "保存",
		"live.noRtc":   "このブラウザはHEVC非対応です。",
		"live.relogin": "再ログインしてください。", "live.notConfigured": "カメラ未設定 — 設定を開いてください。",
		"live.lastError": "直前のエラー: ", "live.saving": "バッファを保存中…",
		"live.downloaded": "ダウンロードしました: ", "live.saveFail": "保存できませんでした: ",
		"save.inactive": "配信が停止しています", "save.empty": "バッファが空です",
		"live.saveErr": "保存エラー",
		"live.battery": "バッテリー", "live.online": "オンライン", "live.offline": "オフライン",
		"live.upgrade":      "新しいファームウェアあり",
		"live.alwaysOn":     "常時配信：カメラはスリープしません。設定でオフにできます。",
		"live.switchFail":   "カメラを切り替えられませんでした。",
		"live.waiting":      "カメラ待機中…",
		"live.reconnecting": "再接続中…",
		"stats.title":       "デバイス", "stats.day": "1日", "stats.week": "1週", "stats.month": "1ヶ月",
		"stats.points":     "点数: %s · %s分ごと",
		"stats.empty":      "バッテリーデータなし — 最初の点は%s分以内に出ます。",
		"stats.fail":       "バッテリー履歴を読み込めません。",
		"stats.poll":       "バックグラウンド確認",
		"stats.pollCustom": "分（1–1440）", "stats.pollSave": "保存",
		"stats.pollBad":   "1–1440分を入力してください。",
		"stats.pollSaved": "バックグラウンド履歴用の間隔を保存しました。ライブ/状態は訪問のたびに更新されます（クラウドAPI、カメラは起動しません）。",
		"stats.pollHint":  "1回のクラウド要求でバッテリー、Wi‑Fi、オンライン、ファームウェア旗を読みます。カメラは起動しません。サイトを開いたときも更新されます。",
		"logs.title":      "ログ", "logs.download": "すべてダウンロード", "logs.limit": "上限 100 MB",
		"logs.shown": " · 画面 %.1f KB / 全体 %.1f MB", "logs.empty": "(空)",
		"logs.lez": "ブリッジ (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "ブリッジのログ", "logs.on": "オン", "logs.off": "オフ",
		"logs.levelHint":    "詳細なブリッジログ（全パケット、約45 MB/日）をlez.logに書き込みます。次の配信開始から有効です。",
		"logs.saved":        "保存しました。次に配信が始まるときに適用されます。",
		"logs.clear":        "ログファイルを消去",
		"logs.clearConfirm": "lez.log、ffmpeg.log、bridge.err を消去しますか？",
		"maint.clearStats":  "バッテリー履歴をリセット",
	},
	"pt": {
		"nav.live": "Ao vivo", "nav.stats": "Dispositivo", "nav.logs": "Logs",
		"nav.setup": "Definições", "nav.logout": "Sair", "lang": "Idioma",
		"login.title": "Entrar", "login.password": "Palavra-passe", "login.submit": "Entrar",
		"login.wrong": "Palavra-passe errada.", "login.ratelimit": "Demasiadas tentativas. Aguarde 10 minutos.",
		"init.title": "Palavra-passe do site", "init.hint": "É a palavra-passe da página, não da EZVIZ.",
		"init.password": "Palavra-passe do site", "init.save": "Guardar", "init.min8": "Mínimo 8 caracteres.",
		"error":       "Erro",
		"setup.title": "Definições", "setup.ezviz": "Conta EZVIZ", "setup.email": "E-mail EZVIZ", "setup.ezvizpw": "Palavra-passe EZVIZ",
		"setup.region": "Região", "setup.save": "Guardar e iniciar",
		"setup.devicesHint": "Câmaras no Live — mude lá.",
		"setup.saved":       "Guardado. A câmara está a acordar — vídeo em ~20 s.",
		"setup.sitepw":      "Palavra-passe do site",
		"setup.current":     "Palavra-passe atual", "setup.new": "Nova palavra-passe", "setup.repeat": "Repetir",
		"setup.change": "Alterar palavra-passe", "setup.pwBad": "Palavra-passe atual errada.", "setup.pwShort": "Mínimo 8 caracteres.",
		"setup.pwMismatch": "As palavras-passe não coincidem.", "setup.pwOk": "Palavra-passe alterada.",
		"setup.stream": "Stream", "setup.streamOnDemand": "Sob pedido", "setup.streamAlways": "Contínuo",
		"setup.streamHint":          "Sob pedido: só com o direto aberto, dorme ~10 s. Contínuo: não para, para medir a bateria.",
		"setup.streamSavedOnDemand": "Sob pedido. Dorme ~10 s se ninguém estiver a ver.",
		"setup.streamSavedAlways":   "Contínuo. A câmara fica acordada; veja Bateria.",
		"setup.update":              "Atualizações",
		"setup.updateCheck":         "Verificar atualizações", "setup.upToDate": "Já está atualizado.",
		"setup.updated": "Atualizado.", "setup.updateFail": "Falha na atualização.",
		"setup.updating": "A verificar…", "setup.downloading": "A descarregar…", "setup.installing": "A instalar…", "setup.restarting": "A reiniciar…",
		"live.title": "Ao vivo", "live.save": "Guardar",
		"live.noRtc":   "Este browser não reproduz HEVC.",
		"live.relogin": "Inicie sessão novamente.", "live.notConfigured": "Câmara não configurada — abra Definições.",
		"live.lastError": "Último erro: ", "live.saving": "A guardar buffer…",
		"live.downloaded": "Transferido: ", "live.saveFail": "Não foi guardado: ",
		"save.inactive": "o stream não está ativo", "save.empty": "o buffer está vazio",
		"live.saveErr": "Erro ao guardar",
		"live.battery": "Bateria", "live.online": "online", "live.offline": "offline",
		"live.upgrade":      "há firmware novo",
		"live.alwaysOn":     "Stream contínuo: a câmara não dorme. Desligue em Definições.",
		"live.switchFail":   "Não foi possível mudar de câmara.",
		"live.waiting":      "À espera da câmara…",
		"live.reconnecting": "A ligar novamente…",
		"stats.title":       "Dispositivo", "stats.day": "Dia", "stats.week": "Semana", "stats.month": "Mês",
		"stats.points":     "Amostras: %s · a cada %s min",
		"stats.empty":      "Ainda sem dados de bateria — a primeira amostra aparece em %s minutos.",
		"stats.fail":       "Não foi possível carregar o histórico da bateria.",
		"stats.poll":       "Consulta em segundo plano",
		"stats.pollCustom": "Minutos (1–1440)", "stats.pollSave": "Guardar",
		"stats.pollBad":   "Introduza 1–1440 minutos.",
		"stats.pollSaved": "Intervalo guardado para o histórico em segundo plano. Ao vivo/estado também atualizam em cada visita (API na nuvem, não acorda a câmara).",
		"stats.pollHint":  "Um pedido à nuvem lê bateria, Wi‑Fi, online e o indicador de firmware. Não acorda a câmara. Também atualiza ao abrir o site.",
		"logs.title":      "Logs", "logs.download": "descarregar tudo", "logs.limit": "limite 100 MB",
		"logs.shown": " · %.1f KB no ecrã de %.1f MB", "logs.empty": "(vazio)",
		"logs.lez": "Ponte (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Registo da ponte", "logs.on": "Ligado", "logs.off": "Desligado",
		"logs.levelHint":    "Escreve registos detalhados da ponte (cada pacote, ~45 MB/dia) em lez.log. Aplica-se no próximo arranque do stream.",
		"logs.saved":        "Guardado. Aplica-se quando o stream voltar a arrancar.",
		"logs.clear":        "Limpar ficheiros de log",
		"logs.clearConfirm": "Limpar lez.log, ffmpeg.log e bridge.err?",
		"maint.clearStats":  "Repor histórico da bateria",
	},
}

func T(lang, key string) string {
	if m, ok := catalog[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if s, ok := catalog["en"][key]; ok {
		return s
	}
	return key
}

func normalizeLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, "-_"); i > 0 {
		code = code[:i]
	}
	if _, ok := catalog[code]; ok {
		return code
	}
	return ""
}

func langOf(r *http.Request) string {
	if c, err := r.Cookie(langCookie); err == nil {
		if l := normalizeLang(c.Value); l != "" {
			return l
		}
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if l := normalizeLang(tag); l != "" {
			return l
		}
	}
	return "en"
}

func setLangCookie(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:     langCookie,
		Value:    lang,
		Path:     *basePath + "/",
		MaxAge:   365 * 24 * 3600,
		SameSite: http.SameSiteLaxMode,
	})
}

func localeFor(lang string) string {
	switch lang {
	case "zh":
		return "zh-CN"
	case "pt":
		return "pt-BR"
	default:
		return lang
	}
}

func handleLang(w http.ResponseWriter, r *http.Request) {
	lang := normalizeLang(r.URL.Query().Get("l"))
	if lang == "" {
		lang = "en"
	}
	setLangCookie(w, lang)
	http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
}

func safeNext(next string) string {
	b := *basePath
	if next == "" || strings.Contains(next, "://") || strings.HasPrefix(next, "//") {
		return b + "/login"
	}
	if next == b || strings.HasPrefix(next, b+"/") || strings.HasPrefix(next, b+"?") {
		return next
	}
	return b + "/login"
}

func langBar(r *http.Request) string {
	cur := langOf(r)
	var b strings.Builder
	b.WriteString(`<form class="langbar" method="get" action="` + *basePath + `/lang">`)
	b.WriteString(`<input type="hidden" name="next" value="` + html.EscapeString(r.URL.RequestURI()) + `">`)
	b.WriteString(`<label>` + html.EscapeString(T(cur, "lang")) + `</label>`)
	b.WriteString(`<select name="l" onchange="this.form.submit()">`)
	for _, l := range languages {
		sel := ""
		if l.Code == cur {
			sel = " selected"
		}
		b.WriteString(`<option value="` + l.Code + `"` + sel + `>` + l.Name + `</option>`)
	}
	b.WriteString(`</select></form>`)
	return b.String()
}
