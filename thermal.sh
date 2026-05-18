# Run this in a screen/tmux session
while true; do
  echo "$(date) GPU:$(nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader)°C" >> /var/log/gpu-temp.log
  sleep 5
done &
