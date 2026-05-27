const electronConsole = { log: console.log.bind(console), warn: console.warn.bind(console), error: console.error.bind(console) };

const electronLogger = {
  info: (message, data) => {
    if (data !== undefined && data !== null) {
      electronConsole.log(`[electron] [INFO] ${message}`, data);
    } else {
      electronConsole.log(`[electron] [INFO] ${message}`);
    }
  },
  warn: (message, data) => {
    if (data !== undefined && data !== null) {
      electronConsole.warn(`[electron] [WARN] ${message}`, data);
    } else {
      electronConsole.warn(`[electron] [WARN] ${message}`);
    }
  },
  error: (message, data) => {
    if (data !== undefined && data !== null) {
      electronConsole.error(`[electron] [ERROR] ${message}`, data);
    } else {
      electronConsole.error(`[electron] [ERROR] ${message}`);
    }
  },
};

module.exports = { electronLogger };
