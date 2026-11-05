# Event destinations

Organization administrators register and test webhook destinations. Settings and credentials are scoped to the organization; project availability and destination usage are checked before updates and deletion. Migration 055 adds destination and subscription storage, with delivery references used by usage checks and counters.

Projects and agents can subscribe to available organization destinations. Subscription filters match event type, scope and environment; deleting a project or agent cleans up its subscriptions. Delivery workers and history are introduced in the next layer.
