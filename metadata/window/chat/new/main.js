(() => {
    const { services, handlers } = context;
    if (services && services.chat) {
        handlers.chat = services.chat;
    }
    if (services && services.approval) {
        handlers.approval = services.approval;
    }

    return {};
})();
