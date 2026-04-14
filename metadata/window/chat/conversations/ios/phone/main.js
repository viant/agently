(() => {
  // Bridge global services.chat into handlers so that
  // YAML events can resolve handler names like "chat.submitMessage".
  const { services, handlers } = context;
  if (services && services.chat) {
    handlers.chat = services.chat;
  }

  return {};
})();
