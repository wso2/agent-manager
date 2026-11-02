# Event destinations

Organization administrators register and test webhook destinations. Settings and credentials are scoped to the organization; project availability and destination usage are checked before updates and deletion. The shared migration schema reserves subscription and delivery storage for subsequent layers.

Projects and agents can subscribe to available organization destinations. Subscription filters match event type, scope and environment; deleting a project or agent cleans up its subscriptions. Delivery workers and history are introduced in the next layer.
