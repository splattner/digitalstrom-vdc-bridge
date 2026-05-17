# MQTT Broker plugin

The MQTT Broker plugin manages a single connection to an MQTT broker and makes it available to other plugins — currently **Tasmota** and **Zigbee2MQTT**. It does not discover any devices on its own.

> **When do I need this?** Only when you also want to use the Tasmota or Zigbee2MQTT plugin. If you are only using Home Assistant or WLED you do not need an MQTT Broker plugin.

---

## Setup walkthrough

**Step 1 — Know your broker.**  
You need the hostname (or IP address) and port of your MQTT broker. Common setups:

- **Mosquitto on the same Docker host:** use the service name (e.g. `mosquitto`) if in the same Docker network, or `localhost` / the host IP otherwise.
- **Home Assistant Mosquitto add-on:** use `homeassistant.local` (or the HA host IP) on port `1883`.
- **A cloud broker (HiveMQ, EMQX Cloud, etc.):** use the provided hostname, port `8883`, and enable TLS.

**Step 2 — Open the Plugins page** and click **+**.

**Step 3 — Choose MQTT Broker** from the plugin picker.

**Step 4 — Fill in the form:**

| Field | What to enter |
|---|---|
| **Plugin ID** | A short unique name, e.g. `mqtt`. Other plugins will reference this ID. |
| **Broker host** | Hostname or IP of your MQTT broker |
| **Broker port** | `1883` for plain MQTT, `8883` for TLS |
| **Use TLS** | Enable if your broker requires TLS |
| **Username / Password** | Leave blank if the broker does not require authentication |

**Step 5 — Click Save.** The status badge should change to **connected** within a few seconds.

---

## Configuration reference

| Field | Type | Default | Description |
|---|---|---|---|
| `host` | string | — | Broker hostname or IP address. **Required.** |
| `port` | integer | `1883` | TCP port. Use `8883` for TLS. |
| `tls` | bool | `false` | Enable TLS encryption |
| `tlsInsecure` | bool | `false` | Skip TLS certificate verification. Only for testing. |
| `caCert` | string (PEM) | — | Optional CA certificate to verify the broker's TLS certificate |
| `clientId` | string | `vdcgo` | MQTT client identifier |
| `username` | string | — | Authentication username |
| `password` | password | — | Authentication password |
| `keepalive` | integer (seconds) | `60` | MQTT keepalive interval |
| `cleanSession` | bool | `true` | Start with a clean MQTT session |

---

## Troubleshooting

**Status stays `connecting`**  
- Check the broker hostname and port.
- Make sure the broker is running and reachable from the machine running the bridge.
- Check the plugin's event log for a specific error message.

**Authentication error**  
- Verify the username and password.
- Some brokers (e.g. the HA Mosquitto add-on) require you to create a dedicated user in the broker config.

**TLS errors**  
- If the broker uses a self-signed certificate, either paste the CA certificate into the `caCert` field or (for testing only) enable `tlsInsecure`.
