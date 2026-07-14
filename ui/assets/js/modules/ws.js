// WebSocket lifecycle (synapp2 pattern).
// For research-tree we use polling as primary transport
// but keep the WS interface for forward compatibility.

export function setupWS({ onOpen, onClose, onError, onMessage, onStateChange } = {}) {
  let ws = null;
  let reconnectTimer = null;
  let stopped = false;
  const RECONNECT_MS = 3000;
  const url = `ws://${location.host}/ws`;

  function connect() {
    if (stopped) return;
    try {
      ws = new WebSocket(url);
    } catch (err) {
      scheduleReconnect();
      return;
    }
    ws.onopen = () => {
      onStateChange && onStateChange('connected');
      onOpen && onOpen();
    };
    ws.onclose = () => {
      onStateChange && onStateChange('disconnected');
      onClose && onClose();
      if (!stopped) scheduleReconnect();
    };
    ws.onerror = (err) => {
      onError && onError(err);
    };
    ws.onmessage = (ev) => {
      onMessage && onMessage(ev.data);
    };
  }

  function scheduleReconnect() {
    if (stopped) return;
    clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(connect, RECONNECT_MS);
  }

  const api = {
    send(data) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(typeof data === 'string' ? data : JSON.stringify(data));
      }
    },
    stop() {
      stopped = true;
      clearTimeout(reconnectTimer);
      if (ws) { ws.onclose = null; ws.close(); ws = null; }
    },
  };

  connect();
  return api;
}
