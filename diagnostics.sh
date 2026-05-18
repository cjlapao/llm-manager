#!/bin/bash

# Run this in a screen/tmux session
while true; do
  echo "$(date) GPU:$(nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader)°C" >> /var/log/gpu-temp.log
  echo "$(date) | $(awk '/MemAvailable/ {printf "Avail: %d MB", $2/1024}' /proc/meminfo) | $(nvidia-smi --query-gpu=temperature.gpu,memory.used --format=csv,noheader,nounits | awk -F', ' '{printf "GPU: %s°C  CUDA: %s MB", $1, $2}')" >> /var/log/mem-crash.log
  sleep 5
done &
