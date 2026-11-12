# Hotel Booking external agent and trace smoke

The PR workflow runs the sample's local smoke test first, then starts Agent
Manager on Docker Compose from the PR checkout with `amp-dev-stack`.

`platform_smoke.py` runs the sample as an external agent (see
[../README.md](../README.md)). It also starts the sample's Hotel API from this
checkout and a local OpenAI-compatible model stub, which requests the hotel
search tool and only answers once the tool result for this invocation's marker
(`hotel-smoke-<hex>`) comes back. The run passes once the observer returns a
model span containing that marker.

## Local run

Start the stack (`make up` from the repository root), then:

```bash
python3.11 -m venv /tmp/hotel-smoke-env && /tmp/hotel-smoke-env/bin/pip install requests
IDP_CLIENT_SECRET=amp-api-client-secret \
  /tmp/hotel-smoke-env/bin/python test/sample-smoke/hotel_booking/platform_smoke.py \
  --python python3.11 --reports /tmp/hotel-booking-platform
python3.11 -m unittest discover -s test/sample-smoke/hotel_booking -p 'test_*.py' -v
```
