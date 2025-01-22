	res, err := resource.New(ctx,
		resource.WithAttributes(
			attr
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("application", "otel-otlp-go-app"),
		),
	)