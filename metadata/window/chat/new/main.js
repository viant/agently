(() => {
  // Bridge global services.chat into handlers so that YAML events like
  // "chat.submitMessage" and "chat.newConversation" can resolve correctly
  // without requiring each window to duplicate the mapping logic.
  // const { services, handlers } = context;
  // if (services && services.chat) {
  //   handlers.chat = services.chat;
  // }
  return {};
})();
