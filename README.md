# Access Controller Controller

Manages the configuration of RFID access controllers.

- Keycloak users are sync'd to the controller
- Fob swipes are scraped and stored in a postgres database


## How does it work?

The access controller exposes an html forms-based web interface for managing access and viewing card swipes. We simply interact with that interface over http and parse the html without rendering it.

Doing so requires a lot of trickery because the interface is stateful — some operations must happen in a particular order. So there are a number of... unconventional concurrency controls throughout this project.


## Usage

Builds are available as container images hosted by the Github registry.

Provide configuration in environment variables:

- `ACCESS_CONTROL_HOST`: hostname:port of the access controller's web interface
- `POSTGRES_HOST`, `POSTGRES_USER`, `POSTGRES_PASSWORD`: Postgres configuration for fob swipe reporting
- `KEYCLOAK_URL`, `KEYCLOAK_USER`, `KEYCLOAK_PASSWORD`, `KEYCLOAK_REALM`: Keycloak connection info
- `AUTHORIZED_GROUP_ID`: the UUID of the Keycloak group that should be granted building access
- `WEBHOOK_ADDR`: Address to serve the Keycloak webhook server on
- `CALLBACK_URL`: The URL that Keycloak should use when sending webhooks

All configuration is optional. Omitting a value will disable the corresponding functionality.


### Keycloak Webhooks

To avoid waiting for the next polling cycle, this service accepts webhooks from Keycloak using the [keycloak-events plugin](https://github.com/p2-inc/keycloak-events).

When `WEBHOOK_ADDR` and `CALLBACK_URL` are set, the service will register its own webhook with Keycloak. Beware that old webhooks will not be cleaned up if the `CALLBACK_URL` changes.
