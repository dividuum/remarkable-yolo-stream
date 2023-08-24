let canvas = document.getElementById("canvas")
let ctx = canvas.getContext("2d")

async function update() {
  try {
    const response = await fetch('/raw')
    const stream = response.body
    const reader = stream.getReader()

    let imageData = ctx.createImageData(canvas.width, canvas.height)
    let offset = 0

    let chunks = []
    while (true) {
      const {done, value} = await reader.read()
      if (done)
        break
      chunks.push(value)
    }
    reader.releaseLock()

    let length = 0
    chunks.forEach(chunk => { length += chunk.length })

    let frame = new Uint8Array(length)
    let frame_offset = 0
    chunks.forEach(chunk => {
      frame.set(chunk, frame_offset)
      frame_offset += chunk.length
    })
    for (let i = 0; i < frame.length; i+=2) {
      const a = frame[i], b = frame[i+1]
      // XXX: what's in a!?
      if (b == 0x52) {
        imageData.data[offset+0] = 255
        imageData.data[offset+1] = 0
        imageData.data[offset+2] = 0
        imageData.data[offset+3] = 255
      } else if (b == 0x8c) {
        imageData.data[offset+0] = 255
        imageData.data[offset+1] = 0
        imageData.data[offset+2] = 255
        imageData.data[offset+3] = 255
      } else if (b == 0x9c) {
        imageData.data[offset+0] = 0
        imageData.data[offset+1] = 0
        imageData.data[offset+2] = 255
        imageData.data[offset+3] = 255
      } else if (b == 0xb5) {
        imageData.data[offset+0] = 0
        imageData.data[offset+1] = 255
        imageData.data[offset+2] = 0
        imageData.data[offset+3] = 255
      } else if (b == 0xd6) {
        imageData.data[offset+0] = 255
        imageData.data[offset+1] = 253
        imageData.data[offset+2] = 84
        imageData.data[offset+3] = 255
      } else {
        imageData.data[offset+0] = b
        imageData.data[offset+1] = b
        imageData.data[offset+2] = b
        imageData.data[offset+3] = 255
      }
      offset += 4
    }
    ctx.putImageData(imageData, 0, 0)
    setTimeout(update, 100)
  } catch (e) {
    ctx.fillStyle = "red"
    ctx.fillRect(0, 0, canvas.width, canvas.height)
  }
}

update()
