(() => {
    const { services, handlers } = context;
    if (services && services.chat) {
        handlers.chat = services.chat;
    }

    return {};
})();
