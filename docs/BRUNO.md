# Bruno Collection

The `bruno/` directory is a [Bruno](https://www.usebruno.com/) collection for exercising the API by hand or as a smoke test. It uses the OpenCollection YAML format, which needs Bruno 3.0 or later.

## Prerequisites

1. Start the stack:

   ```sh
   docker compose up --build
   ```

2. Install one of:
   - The Bruno desktop app, to send requests by hand.
   - Node.js, to run the collection with the Bruno CLI through `npx`.

## Requests

| Request | Expected | What it shows |
| --- | --- | --- |
| Appointments / Book appointment | 201 | Books the generated slot for the generated trainer |
| Appointments / Book same slot | 409 | A second booking of the same slot is rejected |
| Appointments / Get availability | 200 | The booked slot is missing from the surrounding hour |
| Appointments / List appointments | 200 | The trainer's booking is listed |
| Errors / Invalid trainer id | 400 | Path IDs must be UUIDs |
| Errors / Weekend slot | 422 | Bookings must be Monday–Friday |
| Errors / Range over 30 days | 400 | Availability ranges are capped at 30 days |
| Health | 200 | The server is up |
| Reset variables | 200 | Clears generated variables (see below) |

Every request has tests, so a passing run confirms the expected status and key response fields.

## Using the desktop app

1. Choose **Open Collection** and select the `bruno/` directory.
2. Select the **local** environment, which points at `http://localhost:8080`.
3. Send **Book appointment**, then the other requests in the **Appointments** folder in order.

To run everything at once, use the collection's **Run** option.

## Generated variables

A collection script generates the test data as runtime variables, so no IDs or times need to be edited by hand:

| Variable | Value |
| --- | --- |
| `trainerId`, `userId` | Random UUIDs |
| `slotStart`, `slotEnd` | The next weekday, 17:00–17:30 UTC (9am PDT or 10am PST, so always within business hours) |
| `windowStart`, `windowEnd` | 30 minutes before to 60 minutes after the slot, used by **Get availability** |
| `weekendStart`, `weekendEnd` | The following Saturday at the same time, used by **Weekend slot** |
| `rangeEnd` | 31 days after the slot, used by **Range over 30 days** |
| `appointmentId` | Set by **Book appointment** from the response |

The script generates new values in two cases:

1. **Any request sent when no values exist yet.** Requests can be sent in any order.
2. **Every Book appointment send.** Each booking starts a new scenario with a new trainer and gets a 201. The other requests then use that latest booking.

To see the current values, click **(...)** in the top-right corner and select **Variables**.

## Clearing variables

Runtime variables last until they are cleared or Bruno restarts. To clear them, either:

1. Send **Reset variables**, which calls `bru.deleteAllVars()`.
2. Restart Bruno.

The next request sent after clearing generates a fresh set.

## Running with the CLI

From the `bruno/` directory:

```sh
npx @usebruno/cli run --env local
```

Each CLI run starts with no runtime variables, so repeated runs never collide with earlier data and never need clearing.

Other useful forms:

```sh
# One folder or one request
npx @usebruno/cli run errors --env local
npx @usebruno/cli run appointments/book-appointment.yml --env local

# A different port, such as when the app runs with APP_PORT=18080
npx @usebruno/cli run --env local --env-var baseUrl=http://localhost:18080
```

The CLI runs folders before top-level requests, so **Health** and **Reset variables** run last. This is expected.
