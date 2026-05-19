while true; do
  TEMP=$(nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader,nounits)
  POWER=$(nvidia-smi --query-gpu=power.draw --format=csv,noheader,nounits)
  MEM_USED=$(free -g | awk '/Mem:/{print $3}')
  MEM_TOTAL=$(free -g | awk '/Mem:/{print $2}')
  SWAP_USED=$(free -g | awk '/Swap:/{print $3}')
  echo "$(date +%H:%M:%S) GPU=${TEMP}°C PWR=${POWER}W RAM=${MEM_USED}/${MEM_TOTAL}G SWAP=${SWAP_USED}G" >> ~/thermal_monitor.log
  sleep 5
done &
