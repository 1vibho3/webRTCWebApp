// server.js
const express = require('express');
const path = require('path');
const app = express();
const PORT = process.env.PORT || 3001;

// Serve the built React app
app.use(express.static(path.join(__dirname, 'build')));

app.get('*', (_, res) => {
  res.sendFile(path.join(__dirname, 'build', 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Server listening on port ${PORT}`);
});
