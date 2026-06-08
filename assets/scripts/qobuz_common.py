#!/usr/bin/env python3
import configparser
import hashlib
import os
import sys
import time

import requests

import qobuz_dl
import qobuz_dl.downloader as qobuz_downloader
import qobuz_dl.metadata as qobuz_metadata
from qobuz_dl import qopy
from qobuz_dl.bundle import Bundle
from qobuz_dl.color import GREEN, RED, YELLOW
from qobuz_dl.exceptions import AuthenticationError, IneligibleError, InvalidAppIdError, InvalidAppSecretError
from qobuz_dl.qopy import Client

sys.dont_write_bytecode = True

ERROR_MARKER = "__LOADER21_QOBUZ_ERROR__"
QOBUZ_EMAIL_ENV = "LOADER21_QOBUZ_EMAIL"
QOBUZ_PASSWORD_RAW_ENV = "LOADER21_QOBUZ_PASSWORD_RAW"
QOBUZ_PASSWORD_MD5_ENV = "LOADER21_QOBUZ_PASSWORD_MD5"
QOBUZ_USER_AUTH_TOKEN_ENV = "LOADER21_QOBUZ_USER_AUTH_TOKEN"
QOBUZ_DISABLE_TOKEN_AUTH_ENV = "LOADER21_QOBUZ_DISABLE_TOKEN_AUTH"

QOBUZ_RUNTIME_DEFAULTS = {
    "email": "",
    "password": "",
    "default_folder": "Qobuz Downloads",
    "default_quality": "27",
    "default_limit": "20",
    "no_m3u": "false",
    "albums_only": "false",
    "no_fallback": "false",
    "og_cover": "false",
    "embed_art": "false",
    "no_cover": "false",
    "no_database": "false",
    "folder_format": "{artist} - {album} ({year}) [{bit_depth}B-{sampling_rate}kHz]",
    "track_format": "{tracknumber}. {tracktitle}",
    "smart_discography": "false",
}

QOBUZ_TRACK_DOWNLOAD_ATTEMPTS = 5
QOBUZ_DOWNLOAD_TIMEOUT = (10, 60)
QOBUZ_INTER_TRACK_DELAY = 1.0
QOBUZ_QUALITY_FALLBACKS = (27, 7, 6, 5)


class QobuzScriptError(Exception):
    def __init__(self, message, exit_code=1):
        super().__init__(message)
        self.message = str(message or "").strip()
        self.exit_code = exit_code if isinstance(exit_code, int) else 1


class TokenAwareClient(Client):
    def __init__(self, email, pwd, app_id, secrets):
        token = resolve_user_auth_token_value(pwd)
        if not token:
            super().__init__(email, pwd, app_id, secrets)
            self.auth_mode = "credentials"
            self.refreshed_user_auth_token = ""
            return

        qopy.logger.info(f"{YELLOW}Logging...")
        self.secrets = secrets
        self.id = str(app_id)
        self.session = requests.Session()
        self.session.headers.update(
            {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:83.0) Gecko/20100101 Firefox/83.0",
                "X-App-Id": self.id,
            }
        )
        self.base = "https://www.qobuz.com/api.json/0.2/"
        self.sec = None
        self.auth_mode = "token"
        self.refreshed_user_auth_token = ""
        self.auth_via_token(token)
        self.cfg_setup()

    def auth_via_token(self, token):
        token = str(token or "").strip()
        if not token:
            raise AuthenticationError("Invalid token.")

        user_info = token_login_request(self.session, self.base, self.id, token)
        user = user_info.get("user") or {}
        credential = user.get("credential") or {}
        parameters = credential.get("parameters") or {}
        if not parameters:
            raise IneligibleError("Free accounts are not eligible to download tracks.")

        refreshed_token = str(user_info.get("user_auth_token") or token).strip() or token
        self.uat = refreshed_token
        self.refreshed_user_auth_token = refreshed_token
        self.session.headers.update({"X-User-Auth-Token": self.uat})
        self.label = str(parameters.get("short_label", "") or "").strip()
        if self.label:
            qopy.logger.info(f"{GREEN}Membership: {self.label}")


def qobuz_config_path():
    if os.name == "nt":
        appdata = str(os.environ.get("APPDATA", "")).strip()
        if appdata:
            return os.path.join(appdata, "qobuz-dl", "config.ini")
    return os.path.join(os.path.expanduser("~"), ".config", "qobuz-dl", "config.ini")


def load_client():
    config_path = qobuz_config_path()
    config = configparser.ConfigParser()
    config.read(config_path)

    default = config["DEFAULT"]
    email = str(os.environ.get(QOBUZ_EMAIL_ENV, default.get("email", ""))).strip()
    token = resolve_user_auth_token(default)
    app_id = default.get("app_id", "").strip()
    secrets = [secret for secret in default.get("secrets", "").split(",") if secret]

    if not app_id or not secrets:
        app_id, secrets = refresh_config_tokens(config_path, config)

    if token:
        client = build_token_client(email, token, app_id, secrets, config_path, config)
        return client

    passwords = resolve_password_candidates(default.get("password", ""))
    if not email or not passwords:
        raise QobuzScriptError("qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages.", 11)

    return build_client_with_refresh(email, passwords, app_id, secrets, config_path, config)


def build_client_with_refresh(email, passwords, app_id, secrets, config_path, config):
    refresh_attempted = False

    while True:
        last_auth_error = None
        last_config_error = None

        for candidate in passwords:
            try:
                return Client(email, candidate, app_id, secrets)
            except AuthenticationError as exc:
                last_auth_error = exc
            except (InvalidAppIdError, InvalidAppSecretError) as exc:
                last_config_error = exc
                break

        if refresh_attempted:
            if last_config_error is not None:
                raise last_config_error
            if last_auth_error is not None:
                raise last_auth_error
            raise QobuzScriptError("Authentification Qobuz impossible.", 11)

        app_id, secrets = refresh_config_tokens(config_path, config)
        refresh_attempted = True


def build_token_client(email, token, app_id, secrets, config_path, config):
    refresh_attempted = False

    while True:
        try:
            client = TokenAwareClient(email, token, app_id, secrets)
            persist_user_auth_token(config_path, config, getattr(client, "refreshed_user_auth_token", "") or token)
            return client
        except (InvalidAppIdError, InvalidAppSecretError):
            if refresh_attempted:
                raise
            app_id, secrets = refresh_config_tokens(config_path, config)
            refresh_attempted = True


def token_login_request(session, base_url, app_id, token):
    session.headers.update({"X-User-Auth-Token": str(token or "").strip()})
    last_error = None
    attempts = [
        ("post", {"data": {"app_id": app_id, "extra": "partner"}}),
        ("post", {"params": {"app_id": app_id, "extra": "partner"}}),
        ("post", {"json": {"app_id": app_id, "extra": "partner"}}),
        ("get", {"params": {"app_id": app_id, "extra": "partner"}}),
        ("get", {"params": {"app_id": app_id}}),
        ("get", {}),
    ]

    for method, kwargs in attempts:
        try:
            response = session.request(method, base_url + "user/login", timeout=20, **kwargs)
            if response.status_code == 401:
                last_error = AuthenticationError("Invalid token.")
                continue
            if response.status_code in (400, 403, 404, 405):
                last_error = requests.exceptions.HTTPError(response=response)
                continue
            response.raise_for_status()
            payload = response.json()
            if isinstance(payload, dict) and (payload.get("user_auth_token") or payload.get("user")):
                return payload
            last_error = QobuzScriptError("La reponse Qobuz recue est invalide.", 12)
        except requests.exceptions.RequestException as exc:
            last_error = exc

    if last_error is not None:
        raise last_error
    raise AuthenticationError("Invalid token.")


def refresh_config_tokens(config_path, config):
    bundle = Bundle()
    app_id = str(bundle.get_app_id() or "").strip()
    secrets = [str(secret or "").strip() for secret in bundle.get_secrets().values()]
    secrets = [secret for secret in secrets if secret]
    if not app_id or not secrets:
        raise QobuzScriptError("Impossible d'actualiser la configuration qobuz-dl.", 11)

    config["DEFAULT"]["app_id"] = app_id
    config["DEFAULT"]["secrets"] = ",".join(secrets)
    persist_config(config_path, config)
    return app_id, secrets


def prepare_qobuz_dl_runtime_config():
    config_path = qobuz_config_path()
    config = configparser.ConfigParser()
    config.read(config_path)
    default = config["DEFAULT"]

    dirty = False
    for key, value in QOBUZ_RUNTIME_DEFAULTS.items():
        if not str(default.get(key, "") or "").strip():
            default[key] = value
            dirty = True

    email = str(os.environ.get(QOBUZ_EMAIL_ENV, default.get("email", ""))).strip()
    if email != str(default.get("email", "") or "").strip():
        default["email"] = email
        dirty = True

    token = resolve_user_auth_token(default)
    if token and token != str(default.get("token", "") or "").strip():
        default["token"] = token
        dirty = True

    app_id = str(default.get("app_id", "") or "").strip()
    secrets = [secret for secret in str(default.get("secrets", "") or "").split(",") if str(secret).strip()]
    if not app_id or not secrets:
        refresh_config_tokens(config_path, config)
        dirty = False

    if dirty:
        persist_config(config_path, config)

    return config_path


def install_qobuz_dl_token_patch():
    qobuz_dl.Client = TokenAwareClient
    qobuz_dl.qopy.Client = TokenAwareClient


def install_qobuz_dl_download_patch():
    qobuz_downloader.tqdm_download = resilient_tqdm_download
    qobuz_downloader.Download = ResilientDownload
    qobuz_dl.downloader.tqdm_download = resilient_tqdm_download
    qobuz_dl.downloader.Download = ResilientDownload


class ResilientDownload(qobuz_downloader.Download):
    def download_release(self):
        count = 0
        meta = self.client.get_album_meta(self.item_id)

        if not meta.get("streamable"):
            raise qobuz_downloader.NonStreamable("This release is not streamable")

        if self.albums_only and (
            meta.get("release_type") != "album"
            or meta.get("artist").get("name") == "Various Artists"
        ):
            qobuz_downloader.logger.info(
                f'{qobuz_downloader.OFF}Ignoring Single/EP/VA: {meta.get("title", "n/a")}'
            )
            return

        album_title = qobuz_downloader._get_title(meta)

        format_info = self._get_format(meta)
        file_format, quality_met, bit_depth, sampling_rate = format_info

        if not self.downgrade_quality and not quality_met:
            qobuz_downloader.logger.info(
                f"{qobuz_downloader.OFF}Skipping {album_title} as it doesn't meet quality requirement"
            )
            return

        qobuz_downloader.logger.info(
            f"\n{qobuz_downloader.YELLOW}Downloading: {album_title}\nQuality: {file_format}"
            f" ({bit_depth}/{sampling_rate})\n"
        )
        album_attr = self._get_album_attr(
            meta, album_title, file_format, bit_depth, sampling_rate
        )
        folder_format, track_format = qobuz_downloader._clean_format_str(
            self.folder_format, self.track_format, file_format
        )
        sanitized_title = qobuz_downloader.sanitize_filepath(folder_format.format(**album_attr))
        dirn = os.path.join(self.path, sanitized_title)
        os.makedirs(dirn, exist_ok=True)

        if self.no_cover:
            qobuz_downloader.logger.info(f"{qobuz_downloader.OFF}Skipping cover")
        else:
            qobuz_downloader._get_extra(
                meta["image"]["large"], dirn, og_quality=self.cover_og_quality
            )

        if "goodies" in meta:
            try:
                qobuz_downloader._get_extra(meta["goodies"][0]["url"], dirn, "booklet.pdf")
            except Exception:
                pass

        media_numbers = [track["media_number"] for track in meta["tracks"]["items"]]
        is_multiple = True if len([*{*media_numbers}]) > 1 else False
        for track in meta["tracks"]["items"]:
            if count > 0:
                time.sleep(QOBUZ_INTER_TRACK_DELAY)
            parse = self.client.get_track_url(track["id"], fmt_id=self.quality)
            if "sample" not in parse and parse["sampling_rate"]:
                is_mp3 = True if int(self.quality) == 5 else False
                self._download_and_tag(
                    dirn,
                    count,
                    parse,
                    track,
                    meta,
                    False,
                    is_mp3,
                    track["media_number"] if is_multiple else None,
                )
            else:
                qobuz_downloader.logger.info(f"{qobuz_downloader.OFF}Demo. Skipping")
            count = count + 1
        qobuz_downloader.logger.info(f"{qobuz_downloader.GREEN}Completed")

    def _download_and_tag(
        self,
        root_dir,
        tmp_count,
        track_url_dict,
        track_metadata,
        album_or_track_metadata,
        is_track,
        is_mp3,
        multiple=None,
    ):
        try:
            initial_url = str(track_url_dict.get("url") or "").strip()
            if not initial_url:
                raise KeyError("url")
        except KeyError:
            qobuz_downloader.logger.info(f"{qobuz_downloader.OFF}Track not available for download")
            return

        if multiple:
            root_dir = os.path.join(root_dir, f"Disc {multiple}")
            os.makedirs(root_dir, exist_ok=True)

        filename = os.path.join(root_dir, f".{tmp_count:02}.tmp")
        track_title = track_metadata.get("title")
        artist = qobuz_downloader._safe_get(track_metadata, "performer", "name")
        filename_attr = self._get_filename_attr(artist, track_metadata, track_title)
        formatted_path = qobuz_downloader.sanitize_filename(self.track_format.format(**filename_attr))

        requested_quality = int(self.quality)
        requested_extension = ".mp3" if is_mp3 else ".flac"
        requested_final_file = os.path.join(root_dir, formatted_path)[:250] + requested_extension
        if os.path.isfile(requested_final_file):
            qobuz_downloader.logger.info(f"{qobuz_downloader.OFF}{track_title} was already downloaded")
            return

        selected_quality = None
        selected_final_file = ""
        selected_is_mp3 = is_mp3

        for quality in quality_candidates_for_download(requested_quality):
            effective_is_mp3 = quality == 5
            final_extension = ".mp3" if effective_is_mp3 else ".flac"
            final_file = os.path.join(root_dir, formatted_path)[:250] + final_extension

            if quality != requested_quality:
                qobuz_downloader.logger.warning(
                    f"{YELLOW}Download failed repeatedly for {track_title}. Falling back to quality {quality}."
                )

            for attempt in range(1, QOBUZ_TRACK_DOWNLOAD_ATTEMPTS + 1):
                try:
                    current_url = initial_url
                    if quality != requested_quality or attempt > 1:
                        refreshed = self.client.get_track_url(track_metadata["id"], fmt_id=quality)
                        current_url = str(refreshed.get("url") or "").strip()
                        if not current_url:
                            raise ConnectionError("Track download URL unavailable")
                    resilient_tqdm_download(current_url, filename, filename)
                    selected_quality = quality
                    selected_final_file = final_file
                    selected_is_mp3 = effective_is_mp3
                    break
                except retryable_download_errors() as exc:
                    cleanup_qobuz_temp_file(filename)
                    if attempt >= QOBUZ_TRACK_DOWNLOAD_ATTEMPTS:
                        qobuz_downloader.logger.warning(
                            f"{YELLOW}Download attempt {attempt}/{QOBUZ_TRACK_DOWNLOAD_ATTEMPTS} failed for {track_title} at quality {quality}: {exc}"
                        )
                        break
                    wait_seconds = 2 ** (attempt - 1)
                    qobuz_downloader.logger.warning(
                        f"{YELLOW}Download attempt {attempt}/{QOBUZ_TRACK_DOWNLOAD_ATTEMPTS} failed for {track_title} at quality {quality}: {exc}. Retrying in {wait_seconds}s..."
                    )
                    time.sleep(wait_seconds)

            if selected_quality is not None:
                break

        if selected_quality is None:
            qobuz_downloader.logger.error(
                f"{RED}Failed to download {track_title} after retries and quality fallback. Skipping track..."
            )
            cleanup_qobuz_temp_file(filename)
            return

        tag_function = qobuz_metadata.tag_mp3 if selected_is_mp3 else qobuz_metadata.tag_flac
        try:
            tag_function(
                filename,
                root_dir,
                selected_final_file,
                track_metadata,
                album_or_track_metadata,
                is_track,
                self.embed_art,
            )
        except Exception as exc:
            qobuz_downloader.logger.error(
                f"{qobuz_downloader.RED}Error tagging the file: {exc}", exc_info=True
            )


def resilient_tqdm_download(url, fname, desc):
    response = requests.get(
        url,
        allow_redirects=True,
        stream=True,
        timeout=QOBUZ_DOWNLOAD_TIMEOUT,
    )
    response.raise_for_status()
    total = int(response.headers.get("content-length", 0))
    download_size = 0
    try:
        with open(fname, "wb") as file_handle, qobuz_downloader.tqdm(
            total=total,
            unit="iB",
            unit_scale=True,
            unit_divisor=1024,
            desc=desc,
            bar_format=qobuz_downloader.CYAN + "{n_fmt}/{total_fmt} /// {desc}",
        ) as bar:
            for data in response.iter_content(chunk_size=1024):
                size = file_handle.write(data)
                bar.update(size)
                download_size += size
    finally:
        response.close()

    if total > 0 and download_size < total:
        raise ConnectionError(
            f"Incomplete download ({download_size}/{total} bytes) for {fname}"
        )


def quality_candidates_for_download(requested_quality):
    try:
        requested_quality = int(requested_quality)
    except (TypeError, ValueError):
        return [requested_quality]

    if requested_quality not in QOBUZ_QUALITY_FALLBACKS:
        return [requested_quality]

    start_index = QOBUZ_QUALITY_FALLBACKS.index(requested_quality)
    return list(QOBUZ_QUALITY_FALLBACKS[start_index:])


def retryable_download_errors():
    return (
        requests.exceptions.ChunkedEncodingError,
        requests.exceptions.ConnectionError,
        requests.exceptions.Timeout,
        requests.exceptions.HTTPError,
        ConnectionError,
        OSError,
    )


def cleanup_qobuz_temp_file(path):
    if not path:
        return
    try:
        if os.path.isfile(path):
            os.remove(path)
    except OSError:
        pass


def persist_config(config_path, config):
    directory = os.path.dirname(config_path)
    if directory:
        os.makedirs(directory, exist_ok=True)
    with open(config_path, "w", encoding="utf-8") as configfile:
        config.write(configfile)


def persist_user_auth_token(config_path, config, token):
    token = str(token or "").strip()
    if not token:
        return
    default = config["DEFAULT"]
    if default.get("token", "").strip() == token:
        return
    default["token"] = token
    persist_config(config_path, config)


def resolve_user_auth_token(default):
    if token_auth_disabled():
        return ""

    env_token = str(os.environ.get(QOBUZ_USER_AUTH_TOKEN_ENV, "") or "").strip()
    if env_token:
        return env_token

    token = str(default.get("token", "") or "").strip()
    if token:
        return token

    password = str(default.get("password", "") or "").strip()
    if looks_like_user_auth_token(password):
        return password
    return ""


def resolve_user_auth_token_value(value):
    if token_auth_disabled():
        return ""

    env_token = str(os.environ.get(QOBUZ_USER_AUTH_TOKEN_ENV, "") or "").strip()
    if env_token:
        return env_token
    candidate = str(value or "").strip()
    if looks_like_user_auth_token(candidate):
        return candidate
    return ""


def looks_like_user_auth_token(value):
    candidate = str(value or "").strip()
    return candidate.count(".") == 2 and len(candidate) >= 40


def token_auth_disabled():
    value = str(os.environ.get(QOBUZ_DISABLE_TOKEN_AUTH_ENV, "") or "").strip().lower()
    return value in {"1", "true", "yes", "on"}


def using_token_auth():
    return bool(resolve_user_auth_token_value(""))


def resolve_password_candidates(default_password):
    raw_override = os.environ.get(QOBUZ_PASSWORD_RAW_ENV)
    if raw_override not in (None, ""):
        raw_password = str(raw_override)
        return unique_password_candidates(
            [
                raw_password,
                hashlib.md5(raw_password.encode("utf-8")).hexdigest(),
            ]
        )

    md5_override = str(os.environ.get(QOBUZ_PASSWORD_MD5_ENV, "") or "").strip()
    if md5_override:
        return [md5_override]

    password = str(default_password or "").strip()
    if password:
        return [password]
    return []


def unique_password_candidates(values):
    items = []
    seen = set()
    for value in values:
        candidate = str(value or "")
        if not candidate or candidate in seen:
            continue
        seen.add(candidate)
        items.append(candidate)
    return items


def emit_script_error(message, exit_code=1):
    text = str(message or "").strip()
    if not text:
        text = "Erreur Qobuz inconnue."
    print(ERROR_MARKER + text, file=sys.stderr, flush=True)
    raise SystemExit(exit_code)


def _map_http_error(exc):
    response = getattr(exc, "response", None)
    status_code = getattr(response, "status_code", None)
    if status_code == 401:
        if using_token_auth():
            return "Le token de session Qobuz a ete refuse. Recupere un nouveau user_auth_token depuis une session web Qobuz active puis reessaie."
        return "Qobuz a refuse la connexion. Verifie les identifiants. Si ils fonctionnent dans l'application Qobuz, le blocage peut aussi venir de qobuz-dl ou de l'API Qobuz."
    if status_code == 403:
        return "Qobuz a refuse l'acces a cette ressource."
    if status_code == 404:
        return "La ressource Qobuz demandee est introuvable."
    if status_code == 429:
        return "Qobuz limite temporairement les requetes. Reessaie dans un instant."
    if isinstance(status_code, int) and status_code >= 500:
        return "Qobuz rencontre une erreur temporaire. Reessaie dans un instant."
    return "La requete Qobuz a echoue."


def map_exception_to_message(exc):
    if isinstance(exc, QobuzScriptError):
        return exc.message
    if isinstance(exc, AuthenticationError):
        if using_token_auth():
            return "Le token de session Qobuz a ete refuse via qobuz-dl. Recupere un nouveau user_auth_token depuis le navigateur puis reessaie."
        return "Qobuz a refuse la connexion via qobuz-dl. Verifie les identifiants. Si ils fonctionnent dans l'application Qobuz, le blocage peut aussi venir de qobuz-dl ou d'un changement cote Qobuz."
    if isinstance(exc, IneligibleError):
        return "Le compte Qobuz courant ne permet pas cette operation."
    if isinstance(exc, (InvalidAppIdError, InvalidAppSecretError)):
        return "La configuration qobuz-dl locale est invalide. Reinitialise Qobuz depuis Systeme > Diagnostics."
    if isinstance(exc, requests.exceptions.Timeout):
        return "Qobuz ne repond pas pour le moment. Reessaie dans un instant."
    if isinstance(exc, requests.exceptions.ConnectionError):
        return "Impossible de joindre Qobuz. Verifie la connexion Internet puis reessaie."
    if isinstance(exc, requests.exceptions.HTTPError):
        return _map_http_error(exc)
    if isinstance(exc, requests.exceptions.RequestException):
        return "La requete Qobuz a echoue. Verifie la connexion reseau puis reessaie."
    return ""


def run_with_qobuz_error_handling(main_fn):
    try:
        main_fn()
    except SystemExit as exc:
        code = exc.code
        if code in (None, 0):
            raise
        if isinstance(code, str) and code.strip():
            emit_script_error(code.strip(), 1)

        mapping = {
            2: "Parametres Qobuz invalides.",
            3: "Identifiant Qobuz invalide.",
            10: "qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages.",
            11: "qobuz-dl n'est pas configure. Renseigne email/mot de passe Qobuz dans Reglages.",
            12: "La reponse Qobuz recue est invalide.",
        }
        if isinstance(code, int):
            emit_script_error(mapping.get(code, "La commande Qobuz a echoue."), code)
        emit_script_error("La commande Qobuz a echoue.", 1)
    except Exception as exc:
        message = map_exception_to_message(exc) or "Erreur Qobuz inattendue."
        exit_code = getattr(exc, "exit_code", 1)
        emit_script_error(message, exit_code)
