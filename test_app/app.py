from flask import Flask
import random
import time
import threading

app = Flask(__name__)
busy_lock = threading.Lock()
busy = False

@app.route('/')
def index():
    global busy
    
    # If already busy, reject the request
    with busy_lock:
        if busy:
            return "Service is busy/hung", 503
    
    # Randomly decide to fail or succeed
    if random.random() < 0.2:  # 20% chance to fail / hang
        with busy_lock:
            busy = True
        try:
            time.sleep(10)  # simulate a long-running/hung request
        finally:
            with busy_lock:
                busy = False
                
    return "Hello, I'm alive!", 200

@app.route('/healthz')
def healthz():
    # Report NotReady if currently processing a long request
    with busy_lock:
        if busy:
            return "busy", 503
    return "ok", 200

if __name__ == '__main__':
    # Enable threaded so health endpoint can be served while the app is hung
    app.run(host='0.0.0.0', port=80, threaded=True)
