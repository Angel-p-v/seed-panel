# Seeds

Panel web de solo lectura para seguir qBittorrent desde el móvil. El colector guarda una lectura cada 20 segundos en SQLite y el navegador consulta los datos guardados.

- Pestañas de torrents compartiendo, descargando, parados y con error.
- Búsqueda por nombre y orden por tamaño, ratio, nombre o velocidad de subida.
- Progreso de descarga y detalle de tiempo compartiendo, velocidad de subida y bytes desperdiciados en la sesión.
- Gráficas de subida por hora para hoy y por día desde el lunes. El panel indica la última lectura válida y conserva los datos si qBittorrent deja de responder.

## Instalación de la imagen Docker

Necesitas Docker con Compose y qBittorrent 5.2 o posterior con la Web UI activa. En qBittorrent, abre **Ajustes → Interfaz web → Clave API** y genera una clave si todavía no tienes una.

```bash
git clone https://github.com/Angel-p-v/seed-panel.git
cd seed-panel
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64) arch=arm64 ;;
  *) printf 'Arquitectura no disponible: %s\n' "$arch" >&2; exit 1 ;;
esac
curl -fL -o seed-panel-image.tar.gz "https://github.com/Angel-p-v/seed-panel/releases/download/v0.1.0/seed-panel-v0.1.0-linux-$arch.tar.gz"
docker load -i seed-panel-image.tar.gz
umask 077
mkdir -p data secrets
cp -n .env.example secrets/.env
chmod 600 secrets/.env
```

Editar `secrets/.env` y pegar la clave después de `QBT_API_KEY=`. Docker monta este directorio como solo lectura y el navegador nunca recibe la clave. No guardar la clave en `.env.example`.

El contenedor se conecta a una red Docker existente a la que también debe pertenecer qBittorrent. Ajusta estas variables según tu instalación. Puedes exportarlas antes de ejecutar Compose o ponerlas en un `.env` local en la raíz del proyecto, separado de `secrets/.env`.

| Variable | Valor predeterminado | Uso |
| --- | --- | --- |
| `QBT_NETWORK` | `qbittorrent_default` | Nombre de la red Docker existente |
| `QBT_URL` | `http://qbittorrent:8080` | URL interna de la Web UI desde esa red |
| `PANEL_BIND_IP` | `127.0.0.1` | Dirección del servidor donde escucha el panel |
| `PANEL_IMAGE` | `seed-panel:v0.1.0` | Etiqueta de la imagen cargada |
| `PANEL_PORT` | `8765` | Puerto del panel |
| `PANEL_UID` / `PANEL_GID` | `1000` / `1000` | Identidad del usuario con acceso a los directorios locales |

```bash
export PANEL_UID="$(id -u)" PANEL_GID="$(id -g)"
docker compose up -d --no-build
docker compose logs -f panel
```

En el servidor, abre `http://127.0.0.1:8765`. Para abrirlo desde Android, pon la IP LAN del servidor en `PANEL_BIND_IP` y usa esa IP desde tu wifi o VPN. El panel carece de inicio de sesión propio, así que mantenlo en una red de confianza.

## Datos y mantenimiento

El historial comienza con las primeras lecturas válidas. Usa el huso horario `Europe/Madrid` y conserva hasta 15 días de muestras. La base se guarda en `data/panel.db`. El detalle de bytes desperdiciados puede tardar hasta el siguiente ciclo disponible, ya que el servidor limita las consultas de ficha a una por minuto.

La compilación ejecuta las pruebas de Go y limita el paralelismo para facilitar el despliegue en equipos pequeños.

```bash
docker compose config -q
node --check web/panel.js
docker build -t seed-panel:check .
```
