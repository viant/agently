export const endpoints = {
    appAPI: {
        baseURL: process.env.APP_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    },
    dataAPI: {
        baseURL: process.env.DATA_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    },
    agentlyAPI: {
        baseURL: process.env.DATA_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    }
};  
