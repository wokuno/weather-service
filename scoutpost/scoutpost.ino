#include <Adafruit_BMP3XX.h>
#include <bmp3.h>
#include <bmp3_defs.h>

#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <WebServer.h>
#include <SPIFFS.h> // Include the SPIFFS library
#include <ArduinoJson.h> // Include the ArduinoJson library

// Replace with your network credentials
char ssid[32] = "2023 HGTV Smart Home";
char password[64] = "021c9f868a";

// Server URL to send sensor data
const char* serverUrl = "weather.wokuno.com";
const int serverPort = 443;

WebServer server(80);
Adafruit_BMP3XX bmp;

const int ledPin = 2;  // LED pin number

String uuid = ""; // Variable to store the UUID

void handleRoot() {
  String html = "<html><body>";
  html += "<h1>ESP32 Wi-Fi Setup</h1>";
  html += "<form method='POST' action='/save'>";
  html += "<label for='ssid'>Wi-Fi SSID:</label><br>";
  html += "<input type='text' id='ssid' name='ssid'><br><br>";
  html += "<label for='password'>Wi-Fi Password:</label><br>";
  html += "<input type='password' id='password' name='password'><br><br>";
  html += "<input type='submit' value='Submit'>";
  html += "</form>";
  html += "</body></html>";

  server.send(200, "text/html", html);
}

void handleSave() {
  String newSSID = server.arg("ssid");
  String newPassword = server.arg("password");

  newSSID.toCharArray(ssid, 32);
  newPassword.toCharArray(password, 64);

  // Save Wi-Fi credentials to a file
  saveWiFiCredentials();

  server.send(200, "text/html", "<h1>Wi-Fi credentials saved successfully</h1>");
  delay(2000);

  ESP.restart();
}

void setup() {
  pinMode(ledPin, OUTPUT);  // Set LED pin as output

  Serial.begin(115200);

  // Initialize SPIFFS
  if (!SPIFFS.begin(true)) {
    Serial.println("An error occurred while mounting SPIFFS");
    return;
  }

  // Format SPIFFS if necessary
  if (!SPIFFS.exists("/wifi_credentials.txt")) {
    formatSPIFFS();
  }

  // Initialize the BMP388 sensor
  if (!bmp.begin_I2C()) {
    Serial.println("Could not find a valid BMP3 sensor, check wiring!");
    while (1);
  }

  // Check if there are saved Wi-Fi credentials
  if (!loadWiFiCredentials()) {
    // Start the access point if no saved credentials found
    startAccessPoint();
  } else {
    // Connect to the Wi-Fi network
    connectToWiFi();
  }
  
  // Load the UUID
  uuid = loadUUID();
}

void loop() {
  if (WiFi.status() == WL_CONNECTED) {
    // Read temperature and pressure from the BMP388 sensor
    float temperature = bmp.readTemperature();
    float pressure = bmp.readPressure() / 100.0; // Convert to hPa

    // Send the temperature and pressure data to the server
    if (!sendDataToServer(temperature, pressure)) {
      // Failed to send data, blink LED continuously
      // blinkLed(0, 500, 500);
    }

    //    Serial.println(temperature);

    delay(5000);
  } else {
    server.handleClient();
    // Failed to connect to Wi-Fi, blink LED continuously
    blinkLed(0, 500, 500);
  }
}

void startAccessPoint() {
  // Replace with your desired AP credentials
  const char* apSSID = "MyESP32AP";
  const char* apPassword = "password";

  WiFi.softAP(apSSID, apPassword);
  delay(100);

  Serial.print("AP IP address: ");
  Serial.println(WiFi.softAPIP());

  server.on("/", handleRoot);
  server.on("/save", handleSave);
  server.begin();
}

void connectToWiFi() {
  Serial.print("Connecting to Wi-Fi...");

  WiFi.begin(ssid, password);

  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }

  Serial.println();
  Serial.print("Connected to Wi-Fi. IP address: ");
  Serial.println(WiFi.localIP());
}

bool loadWiFiCredentials() {
  // Check if the WiFi credentials file exists
  if (!SPIFFS.exists("/wifi_credentials.txt")) {
    return false;
  }

  // Read the WiFi credentials from the file
  File file = SPIFFS.open("/wifi_credentials.txt", "r");
  if (!file) {
    Serial.println("Failed to open wifi_credentials.txt file for reading");
    return false;
  }

  // Read the SSID
  String ssidLine = file.readStringUntil('\n');
  ssidLine.trim();
  strncpy(ssid, ssidLine.c_str(), sizeof(ssid) - 1);
  ssid[sizeof(ssid) - 1] = '\0';

  // Read the password
  String passwordLine = file.readStringUntil('\n');
  passwordLine.trim();
  strncpy(password, passwordLine.c_str(), sizeof(password) - 1);
  password[sizeof(password) - 1] = '\0';

  Serial.println(ssid);
  Serial.println(password);
  file.close();

  return true;
}

void saveWiFiCredentials() {
  // Save Wi-Fi credentials to a file
  File file = SPIFFS.open("/wifi_credentials.txt", "w");
  if (!file) {
    Serial.println("Failed to open wifi_credentials.txt file for writing");
    return;
  }

  file.println(ssid);
  file.println(password);

  file.close();

  Serial.println("Wi-Fi credentials saved to wifi_credentials.txt");
}

bool sendDataToServer(float temperature, float pressure) {
  WiFiClientSecure client;

  // Disable certificate verification (INSECURE)
  client.setInsecure();

  // Construct the sensor data JSON payload
  DynamicJsonDocument payload(256);
  payload["temperature"] = temperature;
  payload["pressure"] = pressure;
  payload["uuid"] = uuid;

  String payloadStr;
  serializeJson(payload, payloadStr);
  Serial.println(payloadStr);

  if (client.connect(serverUrl, serverPort)) {
    Serial.println("Connected to server");

    client.print("POST /data HTTP/1.1\r\n");
    client.print("Host: ");
    client.println(serverUrl);
    client.println("Content-Type: application/json");
    client.println("Connection: close");
    client.print("Content-Length: ");
    client.println(payloadStr.length());
    client.println();
    client.println(payloadStr);

    while (client.connected()) {
      if (client.available()) {
        String line = client.readStringUntil('\n');
        Serial.println(line);

        // Check for a 308 redirect
        if (line.startsWith("HTTP/1.1 308")) {
          // Extract the new location from the redirect response
          while (line != "\r") {
            line = client.readStringUntil('\n');
            if (line.startsWith("Location: ")) {
              // Extract the hostname from the new location
              String newLocation = line.substring(10);
              newLocation.trim();
              serverUrl = newLocation.c_str(); // Convert to const char*
              break;
            }
          }
        }

        parseUUIDFromResponse(line); // Parse the UUID from the response
      }
    }

    client.stop();
    Serial.println("Connection closed");

    return true;
  } else {
    Serial.println("Failed to connect to server");
    return false;
  }
}

void formatSPIFFS() {
  Serial.println("Formatting SPIFFS...");

  if (SPIFFS.format()) {
    Serial.println("SPIFFS formatted successfully");
  } else {
    Serial.println("SPIFFS formatting failed");
  }
}

void blinkLed(int count, int onTime, int offTime) {
  for (int i = 0; i < count || count == 0; i++) {
    digitalWrite(ledPin, HIGH);
    delay(onTime);
    digitalWrite(ledPin, LOW);
    delay(offTime);
  }
}

String loadUUID() {
  // Check if the UUID file exists
  if (!SPIFFS.exists("/uuid.txt")) {
    return "";
  }

  // Read the UUID from the file
  File file = SPIFFS.open("/uuid.txt", "r");
  if (!file) {
    Serial.println("Failed to open uuid.txt file for reading");
    return "";
  }

  String uuid = file.readStringUntil('\n');
  uuid.trim();
  file.close();

  return uuid;
}

void saveUUID(const String& uuid) {
  // Save the UUID to a file
  File file = SPIFFS.open("/uuid.txt", "w");
  if (!file) {
    Serial.println("Failed to open uuid.txt file for writing");
    return;
  }

  file.println(uuid);
  file.close();

  Serial.println("UUID saved to uuid.txt");
}

void parseUUIDFromResponse(const String& response) {
  DynamicJsonDocument json(256);
  deserializeJson(json, response);

  if (json.containsKey("id")) {
    String newUUID = json["id"].as<String>();
    if (newUUID != uuid) {
      uuid = newUUID;
      saveUUID(uuid); // Save the new UUID
    }
  }
}
