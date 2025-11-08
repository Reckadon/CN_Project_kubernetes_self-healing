from flask import Flask
import random
import time

app = Flask(__name__)

@app.route('/')
def index():
    # Randomly decide to fail or succeed
    if random.random() < 0.2:  # 20% chance to fail
        # Simulate failure: stop responding
        time.sleep(60)  # Sleep for a long time to cause a timeout
    return "Hello, I'm alive!"

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=80)
