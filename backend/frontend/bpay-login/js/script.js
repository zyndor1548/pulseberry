document.addEventListener('DOMContentLoaded', () => {
    const payButton = document.querySelector('.option-button[data-value="pay"]');
    const selectedNumbers = document.getElementById('selected-numbers');

    let paymentId = null; // Retrieve this from the previous page or server
    let amount = null; // Retrieve this from the previous page or server
    let username = "user_123"; // Replace with actual username

    payButton.addEventListener('click', async () => {
        if (!paymentId || !amount) {
            console.error('Payment ID or amount is missing.');
            return;
        }

        const transactionId = `txn_${Date.now()}`;

        try {
            const response = await fetch('https://vortex.aromal.dev/payment', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    id: transactionId,
                    amount: amount,
                    payment_id: paymentId,
                    currency: 'INR',
                    user_id: username
                })
            });

            if (response.ok) {
                const data = await response.json();
                console.log('Payment initiated:', data);

                // WebSocket connection
                const ws = new WebSocket(`ws://vortex.aromal.dev/ws?payment_id=${paymentId}`);

                ws.onopen = () => {
                    console.log('WebSocket connected. Listening for updates...');
                };

                ws.onmessage = (event) => {
                    const message = JSON.parse(event.data);
                    console.log('WebSocket message:', message);

                    if (message.status === 'SUCCESS') {
                        alert('Payment successful!');
                        ws.close();
                    } else if (message.status === 'FAILED') {
                        alert('Payment failed. Please try again.');
                        ws.close();
                    }
                };

                ws.onerror = (error) => {
                    console.error('WebSocket error:', error);
                };

                ws.onclose = () => {
                    console.log('WebSocket connection closed.');
                };
            } else {
                console.error('Failed to initiate payment.');
            }
        } catch (error) {
            console.error('Error during payment:', error);
        }
    });
});