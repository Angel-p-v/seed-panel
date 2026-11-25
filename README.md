# Seeds

Panel web de solo lectura para seguir qBittorrent desde el móvil. El colector guarda una lectura cada 20 segundos en SQLite y el navegador consulta los datos guardados.

- Pestañas **Compartiendo**, **Descargando**, **Parados y errores** y **Tracker**. En **Tracker**, elige un dominio para ver sus torrents en cualquier estado.
- Búsqueda por nombre y orden por tamaño, ratio, nombre o velocidad de subida.
- Progreso de descarga y detalle de tiempo compartiendo, velocidad de subida y bytes desperdiciados en la sesión.
- Gráficas de subida por hora para hoy y por día desde el lunes. Pulsa una barra para ampliar la tarjeta y ver cuánto subió cada torrent en ese intervalo. Pulsa la misma barra otra vez para cerrar el detalle. Las subidas anteriores a la actualización conservan su total.
- El panel indica la última lectura válida y conserva los datos si qBittorrent deja de responder.

## Instalación con Docker

Necesitas Linux de 64 bits (AMD64 o ARM64), Git, Bash, Docker con Compose y qBittorrent 5.2 o posterior con la Web UI activa. En qBittorrent, genera una clave en **Ajustes → Interfaz web → Clave API**.

```bash
git clone https://github.com/Angel-p-v/seed-panel.git
cd seed-panel && bash install.sh
```

El instalador te pide la clave sin mostrarla en pantalla, compila la versión que acabas de clonar y arranca el panel. La clave queda en `secrets/.env`. Si qBittorrent usa otra red o puerto, configura el archivo `.env` como se indica abajo y vuelve a ejecutar `bash install.sh`.

### Conexión con qBittorrent

Si el contenedor de qBittorrent se llama `qbittorrent`, está en la red `qbittorrent_default` y su Web UI escucha en el puerto interno `8080`, puedes usar los valores predeterminados. Si tu instalación difiere, estos comandos ayudan a identificar el contenedor, su red y los puertos publicados:

```bash
docker ps --format '{{.Names}} {{.Ports}}'
docker inspect --format '{{range $name, $net := .NetworkSettings.Networks}}{{println $name}}{{end}}' qbittorrent
docker port qbittorrent
```

Sustituye `qbittorrent` por el nombre real del contenedor. Si el puerto se publica como `8085:8080`, desde la red compartida usa el puerto interno `8080`. Si la propia Web UI escucha en `8085` dentro del contenedor, usa `8085`.

Para guardar los ajustes, crea un archivo `.env` en la raíz del proyecto, al lado de `compose.yaml`. Por ejemplo, para un contenedor llamado `torrent` en la red `media_default`:

```dotenv
QBT_NETWORK=media_default
QBT_URL=http://torrent:8080
```

Si qBittorrent está instalado fuera de Docker o en otro equipo, indica en `QBT_URL` una dirección de su Web UI accesible desde el contenedor. Usa una red Docker existente para el panel. Si no tienes una, créala con `docker network create seed-panel-net` y establece `QBT_NETWORK=seed-panel-net`. Desde el contenedor, `127.0.0.1` apunta al propio panel.

| Variable | Valor predeterminado | Uso |
| --- | --- | --- |
| `QBT_NETWORK` | `qbittorrent_default` | Nombre de la red Docker existente |
| `QBT_URL` | `http://qbittorrent:8080` | URL interna de la Web UI desde esa red |
| `PANEL_BIND_IP` | `127.0.0.1` | Dirección del servidor donde escucha el panel |
| `PANEL_IMAGE` | `seed-panel:local` con el instalador | Etiqueta de la imagen Docker |
| `PANEL_PORT` | `8765` | Puerto del panel |
| `PANEL_UID` / `PANEL_GID` | `1000` / `1000` | Identidad del usuario con acceso a los directorios locales |

```bash
docker compose logs -f panel
```

### Acceso al panel

- **Desde el propio servidor.** Abre `http://127.0.0.1:8765` con la configuración predeterminada.
- **Desde el móvil por wifi o por una VPN con acceso a tu LAN.** Pon la IP LAN del servidor en `PANEL_BIND_IP` y abre esa misma IP con el puerto `8765` desde el móvil.
- **Desde una VPN que da al servidor una IP propia.** Pon esa IP en `PANEL_BIND_IP` para acceder por la VPN.

Si el puerto está ocupado, cambia `PANEL_PORT` y usa el nuevo puerto en la URL. Tras cambiar variables de Compose, vuelve a ejecutar `docker compose up -d --no-build` desde el mismo directorio. El panel carece de inicio de sesión propio, así que mantenlo en una red de confianza.

### Si algo falla

| Síntoma | Qué comprobar |
| --- | --- |
| Docker indica que la red externa no existe | Comprueba el nombre con `docker network ls` y corrige `QBT_NETWORK`. |
| El panel indica que falta la clave | Comprueba que esté en `secrets/.env` y que `PANEL_UID` / `PANEL_GID` correspondan al usuario que creó el archivo. |
| La API devuelve HTTP 401 o 403 | Comprueba que la clave siga vigente y que `QBT_URL` apunte a la Web UI correcta. Si la Web UI valida los nombres de host, revisa que acepte el utilizado en la URL. |
| El panel indica que qBittorrent no responde | Revisa el nombre del contenedor, la red compartida y el puerto interno. |
| SQLite da un error de permisos | Comprueba que `data/` pertenezca al usuario configurado para el panel y que pueda escribir ahí. |
| Funciona en el servidor pero no en el móvil | Revisa `PANEL_BIND_IP`, el firewall y, si estás fuera, que tu VPN tenga ruta hasta esa IP. |

## Datos y mantenimiento

El historial comienza con las primeras lecturas válidas. Usa el huso horario `Europe/Madrid` y conserva hasta 15 días de muestras. La base se guarda en `data/panel.db`. El detalle de bytes desperdiciados puede tardar hasta el siguiente ciclo disponible, ya que el servidor limita las consultas de ficha a una por minuto.

La compilación ejecuta las pruebas de Go y limita el paralelismo para facilitar el despliegue en equipos pequeños.

```bash
docker compose config -q
node --check web/panel.js
docker build -t seed-panel:check .
```
