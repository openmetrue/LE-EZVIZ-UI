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
		"nav.live": "Live", "nav.rec": "Archive", "nav.stats": "Battery", "nav.logs": "Logs",
		"nav.setup": "Settings", "nav.logout": "Log out", "lang": "Language",
		"login.title": "Sign in", "login.password": "Password", "login.submit": "Sign in",
		"login.wrong": "Wrong password.", "login.ratelimit": "Too many attempts. Wait 10 minutes.",
		"init.title": "Set site password", "init.hint": "This is the page password, not your EZVIZ account.",
		"init.password": "Site password", "init.save": "Save", "init.min8": "At least 8 characters.",
		"error":       "Error",
		"setup.title": "Settings", "setup.email": "EZVIZ account email", "setup.ezvizpw": "EZVIZ password",
		"setup.serial": "Device serial", "setup.region": "Region", "setup.save": "Save and start",
		"setup.saved":  "Saved. The camera is waking — video in about 20 seconds.",
		"setup.sitepw": "Site login password", "setup.sitepwHint": "This is the page password, not EZVIZ.",
		"setup.current": "Current password", "setup.new": "New password", "setup.repeat": "Again",
		"setup.change": "Change password", "setup.token": "Machine token:",
		"setup.pwBad": "Current site password is wrong.", "setup.pwShort": "New password must be at least 8 characters.",
		"setup.pwMismatch": "New password and confirmation do not match.", "setup.pwOk": "Site password changed.",
		"live.title": "Live", "live.save": "Save", "live.share": "Share",
		"live.noHls":   "This browser cannot play HLS. Open in Safari.",
		"live.relogin": "Please sign in again.", "live.notConfigured": "Camera is not set up — open Settings.",
		"live.lastError": "Last error: ", "live.saving": "Saving buffer…",
		"live.saved": "Saved: ", "live.seeArchive": " — see Archive", "live.saveFail": "Could not save: ",
		"save.inactive": "stream is not active", "save.empty": "buffer is empty",
		"live.saveErr": "Save failed", "live.copied": "Link copied",
		"live.battery": "Battery", "live.online": "online", "live.offline": "offline",
		"live.upgrade": "firmware update available", "live.waking": "Camera is waking up…",
		"rec.title": "Archive", "rec.total": "Total: %s MB of 1024 MB",
		"rec.clearAll":  "Delete all saved videos",
		"rec.clearHint": "This removes downloaded clips from Archive (up to 1 GB on disk). Live video and logs are not touched.",
		"rec.confirm":   "Delete ALL saved videos from Archive? The live camera is not affected.",
		"stats.title":   "Battery", "stats.day": "Day", "stats.week": "Week", "stats.month": "Month",
		"stats.points": "Samples: %s · every 15 minutes",
		"stats.empty":  "No battery data yet — the first sample appears within 15 minutes.",
		"stats.fail":   "Could not load battery history.",
		"logs.title":   "Logs", "logs.download": "download all", "logs.limit": "archive limit 100 MB",
		"logs.shown": " · %.1f KB on screen of %.1f MB", "logs.empty": "(empty)",
		"logs.lez": "Bridge (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Bridge log level", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug logs every stream packet (~45 MB/day) and session URLs. Applies on the next stream start.",
		"logs.saved":     "Log level saved. It applies when the stream next starts.",
		"logs.clear":     "Clear log files", "logs.clearHint": "Empties lez.log, ffmpeg.log and bridge.err. The systemd journal is not wiped.",
		"logs.clearConfirm": "Clear lez.log, ffmpeg.log and bridge.err?",
		"maint.title":       "Maintenance", "maint.summary": "Logs: %.1f MB of 100 MB · Battery samples: %s · ",
		"maint.read": "open Logs", "maint.clearLogs": "Clear log files", "maint.clearStats": "Reset battery history",
		"maint.statsHint": "Clears the battery chart only — not videos and not log files.",
		"maint.clearRec":  "Delete all saved videos", "maint.confirmRec": "Delete ALL saved videos from Archive?",
	},
	"ru": {
		"nav.live": "Стрим", "nav.rec": "Архив", "nav.stats": "Батарея", "nav.logs": "Логи",
		"nav.setup": "Настройки", "nav.logout": "Выйти", "lang": "Язык",
		"login.title": "Вход", "login.password": "Пароль", "login.submit": "Войти",
		"login.wrong": "Неверный пароль.", "login.ratelimit": "Слишком много попыток. Подождите 10 минут.",
		"init.title": "Задайте пароль сайта", "init.hint": "Это пароль входа на страницу камеры (не EZVIZ).",
		"init.password": "Пароль сайта", "init.save": "Сохранить", "init.min8": "Минимум 8 символов.",
		"error":       "Ошибка",
		"setup.title": "Настройки", "setup.email": "Email аккаунта EZVIZ", "setup.ezvizpw": "Пароль EZVIZ",
		"setup.serial": "Серийник устройства", "setup.region": "Регион", "setup.save": "Сохранить и запустить",
		"setup.saved":  "Сохранено. Камера просыпается — видео появится через ~20 секунд.",
		"setup.sitepw": "Пароль входа на сайт", "setup.sitepwHint": "Это пароль страницы (не EZVIZ).",
		"setup.current": "Текущий пароль", "setup.new": "Новый пароль", "setup.repeat": "Ещё раз",
		"setup.change": "Сменить пароль", "setup.token": "Машинный токен:",
		"setup.pwBad": "Неверный текущий пароль сайта.", "setup.pwShort": "Новый пароль — минимум 8 символов.",
		"setup.pwMismatch": "Новый пароль и подтверждение не совпадают.", "setup.pwOk": "Пароль сайта изменён.",
		"live.title": "Стрим", "live.save": "Сохранить", "live.share": "Поделиться",
		"live.noHls":   "Браузер не умеет HLS. Откройте в Safari.",
		"live.relogin": "Нужно войти заново.", "live.notConfigured": "Камера не настроена — откройте Настройки.",
		"live.lastError": "Последняя ошибка: ", "live.saving": "Сохраняю буфер…",
		"live.saved": "Сохранено: ", "live.seeArchive": " — см. Архив", "live.saveFail": "Не сохранилось: ",
		"save.inactive": "стрим не активен", "save.empty": "буфер пуст",
		"live.saveErr": "Ошибка сохранения", "live.copied": "Ссылка скопирована",
		"live.battery": "Батарея", "live.online": "онлайн", "live.offline": "офлайн",
		"live.upgrade": "есть новая прошивка", "live.waking": "Камера просыпается…",
		"rec.title": "Архив", "rec.total": "Всего: %s МБ из 1024 МБ",
		"rec.clearAll":  "Удалить все сохранённые видео",
		"rec.clearHint": "Стирает скачанные ролики во вкладке Архив (до 1 ГБ на диске). Живой стрим и логи не трогает.",
		"rec.confirm":   "Удалить ВСЕ сохранённые видео из Архива? Камера и стрим не пострадают.",
		"stats.title":   "Батарея", "stats.day": "Сутки", "stats.week": "Неделя", "stats.month": "Месяц",
		"stats.points": "Точек: %s · опрос каждые 15 минут",
		"stats.empty":  "Пока нет данных — первая точка появится в течение 15 минут.",
		"stats.fail":   "Не удалось загрузить историю батареи.",
		"logs.title":   "Логи", "logs.download": "скачать целиком", "logs.limit": "лимит архива 100 МБ",
		"logs.shown": " · %.1f КБ на экране из %.1f МБ", "logs.empty": "(пусто)",
		"logs.lez": "Мост (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Подробность логов моста", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug пишет каждый пакет (~45 МБ/день) и URL сессий. Применяется при следующем старте стрима.",
		"logs.saved":     "Уровень сохранён. Сработает, когда стрим запустится снова.",
		"logs.clear":     "Очистить файлы логов", "logs.clearHint": "Обнуляет lez.log, ffmpeg.log и bridge.err. Журнал systemd не трогает.",
		"logs.clearConfirm": "Очистить lez.log, ffmpeg.log и bridge.err?",
		"maint.title":       "Обслуживание", "maint.summary": "Логи: %.1f МБ из 100 МБ · Батарея: %s · ",
		"maint.read": "открыть Логи", "maint.clearLogs": "Очистить файлы логов", "maint.clearStats": "Сбросить историю батареи",
		"maint.statsHint": "Стирает только график батареи — не видео и не файлы логов.",
		"maint.clearRec":  "Удалить все сохранённые видео", "maint.confirmRec": "Удалить ВСЕ сохранённые видео из Архива?",
	},
	"zh": {
		"nav.live": "直播", "nav.rec": "录像", "nav.stats": "电量", "nav.logs": "日志",
		"nav.setup": "设置", "nav.logout": "退出", "lang": "语言",
		"login.title": "登录", "login.password": "密码", "login.submit": "登录",
		"login.wrong": "密码错误。", "login.ratelimit": "尝试次数过多，请等待 10 分钟。",
		"init.title": "设置网站密码", "init.hint": "这是网页密码，不是萤石账号。",
		"init.password": "网站密码", "init.save": "保存", "init.min8": "至少 8 个字符。",
		"error":       "错误",
		"setup.title": "设置", "setup.email": "萤石账号邮箱", "setup.ezvizpw": "萤石密码",
		"setup.serial": "设备序列号", "setup.region": "地区", "setup.save": "保存并启动",
		"setup.saved":  "已保存。摄像机正在唤醒，约 20 秒后出画面。",
		"setup.sitepw": "网站登录密码", "setup.sitepwHint": "这是网页密码，不是萤石密码。",
		"setup.current": "当前密码", "setup.new": "新密码", "setup.repeat": "再输入一次",
		"setup.change": "修改密码", "setup.token": "设备令牌：",
		"setup.pwBad": "当前网站密码不正确。", "setup.pwShort": "新密码至少 8 个字符。",
		"setup.pwMismatch": "两次新密码不一致。", "setup.pwOk": "网站密码已更改。",
		"live.title": "直播", "live.save": "保存", "live.share": "分享",
		"live.noHls":   "浏览器不支持 HLS，请用 Safari 打开。",
		"live.relogin": "请重新登录。", "live.notConfigured": "摄像机未配置 — 请打开设置。",
		"live.lastError": "上次错误：", "live.saving": "正在保存缓冲…",
		"live.saved": "已保存：", "live.seeArchive": " — 见录像", "live.saveFail": "保存失败：",
		"save.inactive": "直播未启动", "save.empty": "缓冲区为空",
		"live.saveErr": "保存出错", "live.copied": "链接已复制",
		"live.battery": "电量", "live.online": "在线", "live.offline": "离线",
		"live.upgrade": "有新固件", "live.waking": "摄像机正在唤醒…",
		"rec.title": "录像", "rec.total": "共 %s MB / 1024 MB",
		"rec.clearAll":  "删除全部已保存录像",
		"rec.clearHint": "删除「录像」页中的文件（磁盘上限 1 GB）。不影响直播和日志。",
		"rec.confirm":   "删除录像页中的全部已保存视频？不会关掉直播。",
		"stats.title":   "电量", "stats.day": "一天", "stats.week": "一周", "stats.month": "一月",
		"stats.points": "采样：%s · 每 15 分钟",
		"stats.empty":  "暂无电量数据，约 15 分钟内出现第一个点。",
		"stats.fail":   "无法加载电量历史。",
		"logs.title":   "日志", "logs.download": "下载全部", "logs.limit": "日志上限 100 MB",
		"logs.shown": " · 屏幕 %.1f KB / 共 %.1f MB", "logs.empty": "（空）",
		"logs.lez": "桥接 (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "桥接日志级别", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug 会记录每个数据包（约 45 MB/天）和会话 URL。下次启动直播时生效。",
		"logs.saved":     "日志级别已保存，将在下次启动直播时生效。",
		"logs.clear":     "清空日志文件", "logs.clearHint": "清空 lez.log、ffmpeg.log 和 bridge.err。systemd 日志不会删除。",
		"logs.clearConfirm": "清空 lez.log、ffmpeg.log 和 bridge.err？",
		"maint.title":       "维护", "maint.summary": "日志：%.1f MB / 100 MB · 电量采样：%s · ",
		"maint.read": "打开日志", "maint.clearLogs": "清空日志文件", "maint.clearStats": "重置电量历史",
		"maint.statsHint": "只清除电量图表，不影响录像和日志文件。",
		"maint.clearRec":  "删除全部已保存录像", "maint.confirmRec": "删除录像页中的全部视频？",
	},
	"es": {
		"nav.live": "En vivo", "nav.rec": "Archivo", "nav.stats": "Batería", "nav.logs": "Registros",
		"nav.setup": "Ajustes", "nav.logout": "Salir", "lang": "Idioma",
		"login.title": "Entrar", "login.password": "Contraseña", "login.submit": "Entrar",
		"login.wrong": "Contraseña incorrecta.", "login.ratelimit": "Demasiados intentos. Espere 10 minutos.",
		"init.title": "Contraseña del sitio", "init.hint": "Es la contraseña de la página, no de EZVIZ.",
		"init.password": "Contraseña del sitio", "init.save": "Guardar", "init.min8": "Mínimo 8 caracteres.",
		"error":       "Error",
		"setup.title": "Ajustes", "setup.email": "Email de EZVIZ", "setup.ezvizpw": "Contraseña EZVIZ",
		"setup.serial": "Número de serie", "setup.region": "Región", "setup.save": "Guardar e iniciar",
		"setup.saved":  "Guardado. La cámara se despierta — vídeo en ~20 s.",
		"setup.sitepw": "Contraseña de la página", "setup.sitepwHint": "No es la de EZVIZ.",
		"setup.current": "Contraseña actual", "setup.new": "Nueva contraseña", "setup.repeat": "Otra vez",
		"setup.change": "Cambiar contraseña", "setup.token": "Token de dispositivo:",
		"setup.pwBad": "Contraseña actual incorrecta.", "setup.pwShort": "Mínimo 8 caracteres.",
		"setup.pwMismatch": "Las contraseñas no coinciden.", "setup.pwOk": "Contraseña cambiada.",
		"live.title": "En vivo", "live.save": "Guardar", "live.share": "Compartir",
		"live.noHls":   "El navegador no reproduce HLS. Ábralo en Safari.",
		"live.relogin": "Vuelva a iniciar sesión.", "live.notConfigured": "Cámara no configurada — abra Ajustes.",
		"live.lastError": "Último error: ", "live.saving": "Guardando búfer…",
		"live.saved": "Guardado: ", "live.seeArchive": " — ver Archivo", "live.saveFail": "No se guardó: ",
		"save.inactive": "el directo no está activo", "save.empty": "el búfer está vacío",
		"live.saveErr": "Error al guardar", "live.copied": "Enlace copiado",
		"live.battery": "Batería", "live.online": "en línea", "live.offline": "fuera de línea",
		"live.upgrade": "hay firmware nuevo", "live.waking": "La cámara se está despertando…",
		"rec.title": "Archivo", "rec.total": "Total: %s MB de 1024 MB",
		"rec.clearAll":  "Borrar todos los vídeos guardados",
		"rec.clearHint": "Elimina los clips de Archivo (hasta 1 GB). No toca el directo ni los registros.",
		"rec.confirm":   "¿Borrar TODOS los vídeos guardados de Archivo? La cámara en vivo no se apaga.",
		"stats.title":   "Batería", "stats.day": "Día", "stats.week": "Semana", "stats.month": "Mes",
		"stats.points": "Muestras: %s · cada 15 minutos",
		"stats.empty":  "Aún no hay datos de batería — la primera muestra llega en 15 minutos.",
		"stats.fail":   "No se pudo cargar el historial de batería.",
		"logs.title":   "Registros", "logs.download": "descargar todo", "logs.limit": "límite 100 MB",
		"logs.shown": " · %.1f KB en pantalla de %.1f MB", "logs.empty": "(vacío)",
		"logs.lez": "Puente (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Nivel de registro del puente", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug registra cada paquete (~45 MB/día) y URLs de sesión. Se aplica al siguiente arranque del directo.",
		"logs.saved":     "Nivel guardado. Se aplica cuando el directo vuelva a arrancar.",
		"logs.clear":     "Vaciar archivos de registro", "logs.clearHint": "Vacía lez.log, ffmpeg.log y bridge.err. No borra el journal de systemd.",
		"logs.clearConfirm": "¿Vaciar lez.log, ffmpeg.log y bridge.err?",
		"maint.title":       "Mantenimiento", "maint.summary": "Registros: %.1f MB de 100 MB · Batería: %s · ",
		"maint.read": "abrir Registros", "maint.clearLogs": "Vaciar archivos de registro", "maint.clearStats": "Reiniciar historial de batería",
		"maint.statsHint": "Solo borra la gráfica de batería, no los vídeos ni los logs.",
		"maint.clearRec":  "Borrar todos los vídeos guardados", "maint.confirmRec": "¿Borrar TODOS los vídeos de Archivo?",
	},
	"de": {
		"nav.live": "Live", "nav.rec": "Archiv", "nav.stats": "Akku", "nav.logs": "Logs",
		"nav.setup": "Einstellungen", "nav.logout": "Abmelden", "lang": "Sprache",
		"login.title": "Anmeldung", "login.password": "Passwort", "login.submit": "Anmelden",
		"login.wrong": "Falsches Passwort.", "login.ratelimit": "Zu viele Versuche. 10 Minuten warten.",
		"init.title": "Seitenpasswort setzen", "init.hint": "Das ist das Seitenpasswort, nicht EZVIZ.",
		"init.password": "Seitenpasswort", "init.save": "Speichern", "init.min8": "Mindestens 8 Zeichen.",
		"error":       "Fehler",
		"setup.title": "Einstellungen", "setup.email": "EZVIZ-E-Mail", "setup.ezvizpw": "EZVIZ-Passwort",
		"setup.serial": "Geräteseriennummer", "setup.region": "Region", "setup.save": "Speichern und starten",
		"setup.saved":  "Gespeichert. Kamera wacht auf — Bild in ~20 Sekunden.",
		"setup.sitepw": "Login-Passwort der Seite", "setup.sitepwHint": "Nicht das EZVIZ-Passwort.",
		"setup.current": "Aktuelles Passwort", "setup.new": "Neues Passwort", "setup.repeat": "Wiederholen",
		"setup.change": "Passwort ändern", "setup.token": "Gerätetoken:",
		"setup.pwBad": "Aktuelles Passwort ist falsch.", "setup.pwShort": "Mindestens 8 Zeichen.",
		"setup.pwMismatch": "Passwörter stimmen nicht überein.", "setup.pwOk": "Passwort geändert.",
		"live.title": "Live", "live.save": "Speichern", "live.share": "Teilen",
		"live.noHls":   "Browser kann HLS nicht. In Safari öffnen.",
		"live.relogin": "Bitte neu anmelden.", "live.notConfigured": "Kamera nicht eingerichtet — Einstellungen öffnen.",
		"live.lastError": "Letzter Fehler: ", "live.saving": "Puffer wird gespeichert…",
		"live.saved": "Gespeichert: ", "live.seeArchive": " — siehe Archiv", "live.saveFail": "Nicht gespeichert: ",
		"save.inactive": "Stream ist nicht aktiv", "save.empty": "Puffer ist leer",
		"live.saveErr": "Speichern fehlgeschlagen", "live.copied": "Link kopiert",
		"live.battery": "Akku", "live.online": "online", "live.offline": "offline",
		"live.upgrade": "neue Firmware", "live.waking": "Kamera wacht auf…",
		"rec.title": "Archiv", "rec.total": "Gesamt: %s MB von 1024 MB",
		"rec.clearAll":  "Alle gespeicherten Videos löschen",
		"rec.clearHint": "Löscht Clips im Archiv (max. 1 GB). Livebild und Logs bleiben.",
		"rec.confirm":   "ALLE gespeicherten Videos im Archiv löschen? Die Livekamera bleibt an.",
		"stats.title":   "Akku", "stats.day": "Tag", "stats.week": "Woche", "stats.month": "Monat",
		"stats.points": "Messwerte: %s · alle 15 Minuten",
		"stats.empty":  "Noch keine Akkudaten — erster Punkt innerhalb von 15 Minuten.",
		"stats.fail":   "Akkuhistorie konnte nicht geladen werden.",
		"logs.title":   "Logs", "logs.download": "alles laden", "logs.limit": "Limit 100 MB",
		"logs.shown": " · %.1f KB auf dem Schirm von %.1f MB", "logs.empty": "(leer)",
		"logs.lez": "Bridge (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Bridge-Loglevel", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug loggt jedes Paket (~45 MB/Tag) und Sitzungs-URLs. Gilt beim nächsten Stream-Start.",
		"logs.saved":     "Loglevel gespeichert. Gilt, wenn der Stream als Nächstes startet.",
		"logs.clear":     "Logdateien leeren", "logs.clearHint": "Leert lez.log, ffmpeg.log und bridge.err. Das systemd-Journal bleibt.",
		"logs.clearConfirm": "lez.log, ffmpeg.log und bridge.err leeren?",
		"maint.title":       "Wartung", "maint.summary": "Logs: %.1f MB von 100 MB · Akku: %s · ",
		"maint.read": "Logs öffnen", "maint.clearLogs": "Logdateien leeren", "maint.clearStats": "Akkuhistorie zurücksetzen",
		"maint.statsHint": "Löscht nur die Akkukurve — keine Videos und keine Logdateien.",
		"maint.clearRec":  "Alle gespeicherten Videos löschen", "maint.confirmRec": "ALLE gespeicherten Videos im Archiv löschen?",
	},
	"fr": {
		"nav.live": "Direct", "nav.rec": "Archive", "nav.stats": "Batterie", "nav.logs": "Journaux",
		"nav.setup": "Réglages", "nav.logout": "Quitter", "lang": "Langue",
		"login.title": "Connexion", "login.password": "Mot de passe", "login.submit": "Connexion",
		"login.wrong": "Mot de passe incorrect.", "login.ratelimit": "Trop de tentatives. Attendez 10 minutes.",
		"init.title": "Mot de passe du site", "init.hint": "Mot de passe de la page, pas du compte EZVIZ.",
		"init.password": "Mot de passe du site", "init.save": "Enregistrer", "init.min8": "8 caractères minimum.",
		"error":       "Erreur",
		"setup.title": "Réglages", "setup.email": "E-mail EZVIZ", "setup.ezvizpw": "Mot de passe EZVIZ",
		"setup.serial": "Numéro de série", "setup.region": "Région", "setup.save": "Enregistrer et démarrer",
		"setup.saved":  "Enregistré. La caméra se réveille — image dans ~20 s.",
		"setup.sitepw": "Mot de passe du site", "setup.sitepwHint": "Ce n’est pas celui d’EZVIZ.",
		"setup.current": "Mot de passe actuel", "setup.new": "Nouveau mot de passe", "setup.repeat": "Encore une fois",
		"setup.change": "Changer le mot de passe", "setup.token": "Jeton machine :",
		"setup.pwBad": "Mot de passe actuel incorrect.", "setup.pwShort": "8 caractères minimum.",
		"setup.pwMismatch": "Les mots de passe ne correspondent pas.", "setup.pwOk": "Mot de passe modifié.",
		"live.title": "Direct", "live.save": "Enregistrer", "live.share": "Partager",
		"live.noHls":   "Ce navigateur ne lit pas HLS. Ouvrez Safari.",
		"live.relogin": "Reconnectez-vous.", "live.notConfigured": "Caméra non configurée — ouvrez Réglages.",
		"live.lastError": "Dernière erreur : ", "live.saving": "Enregistrement du tampon…",
		"live.saved": "Enregistré : ", "live.seeArchive": " — voir Archive", "live.saveFail": "Échec : ",
		"save.inactive": "le direct n'est pas actif", "save.empty": "le tampon est vide",
		"live.saveErr": "Erreur d’enregistrement", "live.copied": "Lien copié",
		"live.battery": "Batterie", "live.online": "en ligne", "live.offline": "hors ligne",
		"live.upgrade": "nouveau firmware", "live.waking": "La caméra se réveille…",
		"rec.title": "Archive", "rec.total": "Total : %s Mo sur 1024 Mo",
		"rec.clearAll":  "Supprimer toutes les vidéos enregistrées",
		"rec.clearHint": "Efface les clips de l’Archive (1 Go max). Le direct et les journaux restent.",
		"rec.confirm":   "Supprimer TOUTES les vidéos enregistrées de l’Archive ? Le direct continue.",
		"stats.title":   "Batterie", "stats.day": "Jour", "stats.week": "Semaine", "stats.month": "Mois",
		"stats.points": "Échantillons : %s · toutes les 15 minutes",
		"stats.empty":  "Pas encore de données batterie — premier point dans 15 minutes.",
		"stats.fail":   "Impossible de charger l’historique batterie.",
		"logs.title":   "Journaux", "logs.download": "tout télécharger", "logs.limit": "limite 100 Mo",
		"logs.shown": " · %.1f Ko à l’écran sur %.1f Mo", "logs.empty": "(vide)",
		"logs.lez": "Pont (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Niveau des journaux du pont", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug journalise chaque paquet (~45 Mo/jour) et les URL de session. Prend effet au prochain démarrage du direct.",
		"logs.saved":     "Niveau enregistré. Il s’appliquera au prochain démarrage du direct.",
		"logs.clear":     "Vider les fichiers journaux", "logs.clearHint": "Vide lez.log, ffmpeg.log et bridge.err. Le journal systemd n’est pas effacé.",
		"logs.clearConfirm": "Vider lez.log, ffmpeg.log et bridge.err ?",
		"maint.title":       "Maintenance", "maint.summary": "Journaux : %.1f Mo / 100 Mo · Batterie : %s · ",
		"maint.read": "ouvrir Journaux", "maint.clearLogs": "Vider les fichiers journaux", "maint.clearStats": "Réinit. historique batterie",
		"maint.statsHint": "Efface uniquement le graphique batterie — pas les vidéos ni les journaux.",
		"maint.clearRec":  "Supprimer toutes les vidéos enregistrées", "maint.confirmRec": "Supprimer TOUTES les vidéos de l’Archive ?",
	},
	"ja": {
		"nav.live": "ライブ", "nav.rec": "録画", "nav.stats": "バッテリー", "nav.logs": "ログ",
		"nav.setup": "設定", "nav.logout": "ログアウト", "lang": "言語",
		"login.title": "ログイン", "login.password": "パスワード", "login.submit": "ログイン",
		"login.wrong": "パスワードが違います。", "login.ratelimit": "試行が多すぎます。10分待ってください。",
		"init.title": "サイトのパスワード", "init.hint": "EZVIZアカウントではなく、このページのパスワードです。",
		"init.password": "サイトのパスワード", "init.save": "保存", "init.min8": "8文字以上。",
		"error":       "エラー",
		"setup.title": "設定", "setup.email": "EZVIZメール", "setup.ezvizpw": "EZVIZパスワード",
		"setup.serial": "シリアル番号", "setup.region": "地域", "setup.save": "保存して開始",
		"setup.saved":  "保存しました。カメラ起動中 — 約20秒で映像が出ます。",
		"setup.sitepw": "サイトのログインパスワード", "setup.sitepwHint": "EZVIZのパスワードではありません。",
		"setup.current": "現在のパスワード", "setup.new": "新しいパスワード", "setup.repeat": "再入力",
		"setup.change": "パスワードを変更", "setup.token": "デバイストークン:",
		"setup.pwBad": "現在のパスワードが違います。", "setup.pwShort": "8文字以上にしてください。",
		"setup.pwMismatch": "パスワードが一致しません。", "setup.pwOk": "パスワードを変更しました。",
		"live.title": "ライブ", "live.save": "保存", "live.share": "共有",
		"live.noHls":   "このブラウザはHLS非対応です。Safariで開いてください。",
		"live.relogin": "再ログインしてください。", "live.notConfigured": "カメラ未設定 — 設定を開いてください。",
		"live.lastError": "直前のエラー: ", "live.saving": "バッファを保存中…",
		"live.saved": "保存しました: ", "live.seeArchive": " — 録画を見る", "live.saveFail": "保存できませんでした: ",
		"save.inactive": "配信が停止しています", "save.empty": "バッファが空です",
		"live.saveErr": "保存エラー", "live.copied": "リンクをコピーしました",
		"live.battery": "バッテリー", "live.online": "オンライン", "live.offline": "オフライン",
		"live.upgrade": "新しいファームウェアあり", "live.waking": "カメラを起動しています…",
		"rec.title": "録画", "rec.total": "合計 %s MB / 1024 MB",
		"rec.clearAll":  "保存した動画をすべて削除",
		"rec.clearHint": "録画タブのファイルを消します（最大1 GB）。ライブとログはそのままです。",
		"rec.confirm":   "録画タブの保存動画をすべて削除しますか？ライブは止まりません。",
		"stats.title":   "バッテリー", "stats.day": "1日", "stats.week": "1週", "stats.month": "1ヶ月",
		"stats.points": "点数: %s · 15分ごと",
		"stats.empty":  "バッテリーデータなし — 最初の点は15分以内に出ます。",
		"stats.fail":   "バッテリー履歴を読み込めません。",
		"logs.title":   "ログ", "logs.download": "すべてダウンロード", "logs.limit": "上限 100 MB",
		"logs.shown": " · 画面 %.1f KB / 全体 %.1f MB", "logs.empty": "(空)",
		"logs.lez": "ブリッジ (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "ブリッジのログレベル", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debugは全パケット（約45 MB/日）とセッションURLを記録します。次の配信開始から有効です。",
		"logs.saved":     "ログレベルを保存しました。次に配信が始まるときに適用されます。",
		"logs.clear":     "ログファイルを消去", "logs.clearHint": "lez.log、ffmpeg.log、bridge.err を空にします。systemdジャーナルは残します。",
		"logs.clearConfirm": "lez.log、ffmpeg.log、bridge.err を消去しますか？",
		"maint.title":       "メンテナンス", "maint.summary": "ログ: %.1f MB / 100 MB · バッテリー: %s · ",
		"maint.read": "ログを開く", "maint.clearLogs": "ログファイルを消去", "maint.clearStats": "バッテリー履歴をリセット",
		"maint.statsHint": "バッテリーグラフだけ消します。動画とログファイルは残します。",
		"maint.clearRec":  "保存した動画をすべて削除", "maint.confirmRec": "録画の保存動画をすべて削除しますか？",
	},
	"pt": {
		"nav.live": "Ao vivo", "nav.rec": "Arquivo", "nav.stats": "Bateria", "nav.logs": "Logs",
		"nav.setup": "Definições", "nav.logout": "Sair", "lang": "Idioma",
		"login.title": "Entrar", "login.password": "Palavra-passe", "login.submit": "Entrar",
		"login.wrong": "Palavra-passe errada.", "login.ratelimit": "Demasiadas tentativas. Aguarde 10 minutos.",
		"init.title": "Palavra-passe do site", "init.hint": "É a palavra-passe da página, não da EZVIZ.",
		"init.password": "Palavra-passe do site", "init.save": "Guardar", "init.min8": "Mínimo 8 caracteres.",
		"error":       "Erro",
		"setup.title": "Definições", "setup.email": "E-mail EZVIZ", "setup.ezvizpw": "Palavra-passe EZVIZ",
		"setup.serial": "Número de série", "setup.region": "Região", "setup.save": "Guardar e iniciar",
		"setup.saved":  "Guardado. A câmara está a acordar — vídeo em ~20 s.",
		"setup.sitepw": "Palavra-passe do site", "setup.sitepwHint": "Não é a da EZVIZ.",
		"setup.current": "Palavra-passe atual", "setup.new": "Nova palavra-passe", "setup.repeat": "Repetir",
		"setup.change": "Alterar palavra-passe", "setup.token": "Token da máquina:",
		"setup.pwBad": "Palavra-passe atual errada.", "setup.pwShort": "Mínimo 8 caracteres.",
		"setup.pwMismatch": "As palavras-passe não coincidem.", "setup.pwOk": "Palavra-passe alterada.",
		"live.title": "Ao vivo", "live.save": "Guardar", "live.share": "Partilhar",
		"live.noHls":   "Este browser não reproduz HLS. Abra no Safari.",
		"live.relogin": "Inicie sessão novamente.", "live.notConfigured": "Câmara não configurada — abra Definições.",
		"live.lastError": "Último erro: ", "live.saving": "A guardar buffer…",
		"live.saved": "Guardado: ", "live.seeArchive": " — ver Arquivo", "live.saveFail": "Não foi guardado: ",
		"save.inactive": "o stream não está ativo", "save.empty": "o buffer está vazio",
		"live.saveErr": "Erro ao guardar", "live.copied": "Ligação copiada",
		"live.battery": "Bateria", "live.online": "online", "live.offline": "offline",
		"live.upgrade": "há firmware novo", "live.waking": "A câmara está a acordar…",
		"rec.title": "Arquivo", "rec.total": "Total: %s MB de 1024 MB",
		"rec.clearAll":  "Apagar todos os vídeos guardados",
		"rec.clearHint": "Remove os clips do Arquivo (até 1 GB). Não mexe no direto nem nos logs.",
		"rec.confirm":   "Apagar TODOS os vídeos guardados do Arquivo? A câmara ao vivo continua.",
		"stats.title":   "Bateria", "stats.day": "Dia", "stats.week": "Semana", "stats.month": "Mês",
		"stats.points": "Amostras: %s · a cada 15 minutos",
		"stats.empty":  "Ainda sem dados de bateria — a primeira amostra aparece em 15 minutos.",
		"stats.fail":   "Não foi possível carregar o histórico da bateria.",
		"logs.title":   "Logs", "logs.download": "descarregar tudo", "logs.limit": "limite 100 MB",
		"logs.shown": " · %.1f KB no ecrã de %.1f MB", "logs.empty": "(vazio)",
		"logs.lez": "Ponte (lez.log)", "logs.ffmpeg": "ffmpeg.log", "logs.bridge": "bridge.err", "logs.ezvizd": "ezvizd (journal)",
		"logs.level": "Nível de log da ponte", "logs.levelInfo": "Info", "logs.levelDebug": "Debug",
		"logs.levelHint": "Debug regista cada pacote (~45 MB/dia) e URLs de sessão. Aplica-se no próximo arranque do stream.",
		"logs.saved":     "Nível guardado. Aplica-se quando o stream voltar a arrancar.",
		"logs.clear":     "Limpar ficheiros de log", "logs.clearHint": "Esvazia lez.log, ffmpeg.log e bridge.err. O journal systemd fica.",
		"logs.clearConfirm": "Limpar lez.log, ffmpeg.log e bridge.err?",
		"maint.title":       "Manutenção", "maint.summary": "Logs: %.1f MB de 100 MB · Bateria: %s · ",
		"maint.read": "abrir Logs", "maint.clearLogs": "Limpar ficheiros de log", "maint.clearStats": "Repor histórico da bateria",
		"maint.statsHint": "Apaga só o gráfico da bateria — não os vídeos nem os logs.",
		"maint.clearRec":  "Apagar todos os vídeos guardados", "maint.confirmRec": "Apagar TODOS os vídeos do Arquivo?",
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
