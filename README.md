# Nest AC Troubleshooter
Checks the Nest API to make sure that your AC isn't just blowing out warm air


## Why?
If your AC is getting old like mine, you might notice that it might be blowing warm air when the Nest says it's cooling the house. In my case, this is because my compressor sometimes doesn't start. If your AC never blows cold air, it might be your system or your wiring; this is mainly meant to be a "turn it off and back on again" solution for intermittent issues.

## What does it do?
  1. Hits Nest API to see if your system is cooling
  2. If your system is cooling, and the temperature goes up, it turns the Nest's mode to off and then back to what it was before
  
## How do I use it?
  1. Register to access the [Nest API](https://codelabs.developers.google.com/codelabs/wwn-api-quickstart/#0) and get an auth token and your thermostat ID
  2. You can either pass in the token and thermostat ID as parameters, or put them in the config.json
  3. Let the system run
