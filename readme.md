# Traefik Umami Feeder Plugin

A [Traefik](https://traefik.io/traefik/) middleware plugin that sends visits to your [Umami](https://umami.is) instance.

It was created as an alternative to [traefik-umami-plugin](https://github.com/1cedsoda/traefik-umami-plugin) and
was inspired by the [Plausible Feeder Traefik Plugin](https://github.com/safing/plausiblefeeder).

## Introduction

This plugin integrates your Traefik-proxied services with Umami, a simple, fast, privacy-focused analytics solution. It
captures basic request information (path, user-agent, referrer, screen size, IP) and forwards it to your Umami instance,
enabling server-side analytics.

Key features:

- Stupidly simple to set up — one middleware can be used for all websites
- Server-Side Tracking, no JS or Cookies bullshit
- Fast and private

## Configuration

### Step 1. Add the plugin to Traefik

Declare the plugin in your Traefik **static configuration**.

```yaml
experimental:
  plugins:
    umami-feeder:
      moduleName: github.com/astappiev/traefik-umami-feeder
      version: v1.4.0 # Replace with the latest version
```

### Step 2. Configure the middleware

Once the plugin is declared, configure it as a middleware in your Traefik **dynamic configuration**.

You can specify which websites to track in two ways:

1. **Manual**: Directly provide a `websites` map, associating hostnames with their Umami Website IDs.
2. **Automatic**: Configure the plugin with your Umami API `umamiToken`, or `umamiUsername` and `umamiPassword`. The
   plugin will then automatically fetch the list of websites and their IDs from your Umami instance.
    * Optionally, use `umamiTeamId` to scope website retrieval to a specific team.
    * Optionally, enable `createNewWebsites` to allow the plugin to create new website entries in Umami if they don't
      already exist.

See the [Middleware Options](#middleware-options) section for detailed configuration options.

```yaml
http:
  middlewares:
    my-umami-middleware:
      plugin:
        umami-feeder:
          umamiHost: "http://umami:3000" # URL of your Umami instance

          # Option 1: Define the list of websites
          # websites:
          #   "example.com": "d4617504-241c-4797-8eab-5939b367b3ad"

          # Option 2: Use Umami credentials to fetch websites
          umamiUsername: "your-umami-username"
          umamiPassword: "your-umami-password"
          # umamiToken: "your-umami-api-token" # Alternative to username/password

          # Optional: allow creation of new websites in Umami
          createNewWebsites: true
```

### Step 3. Attach the middleware to your routers

Apply the [configured middleware](https://doc.traefik.io/traefik/routing/routers/#middlewares_1) to the Traefik routers
you want to track with Umami. This is also done in your **dynamic configuration**.

Remember to use the
correct [provider namespace](https://doc.traefik.io/traefik/providers/overview/#provider-namespace)  (e.g., `@file` if
your middleware is defined in a file, `@docker` if defined via Docker labels).

**Example using Docker labels:**

```yaml
- "traefik.http.routers.whoami.middlewares=my-umami-middleware@file"
```

**Example using a dynamic configuration file (e.g., `dynamic_conf.yml`):**

```yaml
http:
  routers:
    whoami:
      rule: "Host(`example.com`)"
      middlewares:
        - my-umami-middleware@file
```

**Example using static configuration (e.g., `traefik.yml`), by attaching the middleware to an entryPoint to apply it
globally:**

```yaml
entryPoints:
  web:
    http:
      middlewares:
        - my-umami-middleware@file
```

## Middleware Options

| key                 | default         | type       | description                                                                                                                                                                                                  |
|---------------------|-----------------|------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `enabled`           | `true`          | `bool`     | Set to `false` to disable the plugin.                                                                                                                                                                        |
| `debug`             | `false`         | `bool`     | Set to `true` for verbose logging. Useful for troubleshooting as plugins don't inherit Traefik's global log level.                                                                                           |
| `queueSize`         | `1000`          | `int`      | Maximum number of tracking events to queue before sending to the Umami server.                                                                                                                               |
| `umamiHost`         | **required**    | `string`   | URL of your Umami instance, reachable from Traefik (e.g., `http://umami:3000`).                                                                                                                              |
| `umamiToken`        | -               | `string`   | [Umami API Token](https://umami.is/docs/api/authentication) for authenticating with your Umami instance. Use this *or* `umamiUsername`/`umamiPassword`. Required for automatic website fetching or creation. |
| `umamiUsername`     | -               | `string`   | Username for Umami authentication. Use this with `umamiPassword` if not using `umamiToken`. Required for automatic website fetching or creation.                                                             |
| `umamiPassword`     | -               | `string`   | Password for Umami authentication, used in conjunction with `umamiUsername`.                                                                                                                                 |
| `umamiTeamId`       | -               | `string`   | Optional. If using automatic mode, specifies the Umami Team ID to scope website fetching/creation.                                                                                                           |
| `websites`          | -               | `map`      | A map of `hostname: umamiWebsiteID`. Used for manual website configuration or to override/extend websites fetched in automatic mode.                                                                         |
| `createNewWebsites` | `false`         | `bool`     | If `true` and using automatic mode, the plugin will attempt to create a new website entry in Umami if the domain is not found.                                                                               |
| `trackErrors`       | `false`         | `bool`     | If `true`, tracks HTTP errors (status codes >= 400).                                                                                                                                                         |
| `trackAllResources` | `false`         | `bool`     | If `true`, tracks requests for all resources. By default, only requests likely to be page views (e.g., HTML, or no specific extension) are tracked.                                                          |
| `trackExtensions`   | `[see sources]` | `string[]` | A list of specific file extensions to track (e.g., `[".html", ".php"]`).                                                                                                                                     |
| `ignoreUserAgents`  | `[]`            | `string[]` | A list of user-agent substrings. Requests with matching user-agents will be ignored (e.g., `["Googlebot", "Uptime-Kuma"]`). Matching is done using `strings.Contains`.                                       |
| `ignoreURLs`        | `[]`            | `string[]` | A list of regular expressions. Requests with URLs matching any of these patterns will be ignored (e.g., `["/health", "https?://[^/]+/health$"]`). Matched with `regexp.Compile.MatchString`.                 |
| `ignoreIPs`         | `[]`            | `string[]` | A list of IP addresses or CIDR ranges to ignore (e.g., `["127.0.0.1", "10.0.0.1/16"]`). Matched with `netip.ParsePrefix.Contains`.                                                                           |
| `headerIp`          | `X-Real-IP`     | `string`   | The HTTP header to inspect for the client's real IP address, typically used when Traefik is behind another proxy.                                                                                            |

## Contributing

Contributions are welcome! Please feel free to submit a pull request or open an issue.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
