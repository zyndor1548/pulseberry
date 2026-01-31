// ===== REGISTER PAGE LOGIC =====
document.addEventListener("DOMContentLoaded", () => {
    const signupForm = document.getElementById("signup-form");
    
    if (signupForm) {
        const usernameInput = document.getElementById("username");
        const passwordInput = document.getElementById("password");
        const confirmInput = document.getElementById("confirm-password");

        signupForm.addEventListener("submit", (e) => {
            e.preventDefault();

            const username = usernameInput.value.trim();
            const password = passwordInput.value;
            const confirm = confirmInput.value;

            if (password !== confirm) {
                alert("Passwords do not match!");
                return;
            }

            let users = JSON.parse(localStorage.getItem("bp_users")) || [];

            if (users.some(u => u.username === username)) {
                alert("Username already exists!");
                return;
            }

            users.push({ username, password });
            localStorage.setItem("bp_users", JSON.stringify(users));
            localStorage.setItem("bp_username", username);

            window.location.href = "login.html";
        });
    }

    // ===== LOGIN PAGE LOGIC =====
    const loginForm = document.getElementById("login-form");
    
    if (loginForm) {
        loginForm.addEventListener("submit", (e) => {
            e.preventDefault();

            const username = document.getElementById("login-username").value.trim();
            const password = document.getElementById("login-password").value;

            const users = JSON.parse(localStorage.getItem("bp_users")) || [];

            const user = users.find(
                u => u.username === username && u.password === password
            );



            localStorage.setItem("bp_username", username);
            window.location.href = "home.html";
        });
    }

    // ===== HOME PAGE LOGIC (AMOUNT INPUT) =====
    const amountForm = document.getElementById("amount-form");

    if (amountForm) {
        amountForm.addEventListener("submit", async (e) => {
            e.preventDefault();
            const money = document.getElementById("money-input").value;
            const username = localStorage.getItem("bp_username") || "";

            try {
                const response = await fetch('https://vortex.aromal.dev/paymentkey', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ id: username, amount: money })
                });

                if (response.ok) {
                    const data = await response.json();
                    const paymentId = data.paymentId || data.payment_id;
                    
                    // Store payment details in localStorage
                    localStorage.setItem("bp_paymentId", paymentId);
                    localStorage.setItem("bp_amount", money);
                    
                    console.log(`Payment ID: ${paymentId}`);

                    // Redirect to buttons.html on success
                    window.location.href = 'buttons.html';
                } else {
                    console.error('Failed to fetch payment key');
                    alert('Failed to process payment. Please try again.');
                }
            } catch (error) {
                console.error('Error:', error);
                alert('Error: ' + error.message);
            }
        });
    }

    // ===== BUTTONS PAGE LOGIC =====
    const selectedNumbers = document.getElementById("selected-numbers");
    const optionButtons = document.querySelectorAll(".option-button");
    let numberSequence = '';

    optionButtons.forEach(button => {
        button.addEventListener('click', () => {
            const value = button.getAttribute('data-value');

            if (value === 'back') {
                numberSequence = numberSequence.slice(0, -1);
            } else if (value !== 'pay') {
                numberSequence += value;
            }

            if (selectedNumbers) {
                selectedNumbers.textContent = numberSequence || 'Choose an Option';
            }
        });
    });

    // Pay button logic for payment
    const payButton = document.querySelector('.option-button[data-value="pay"]');
    
    if (payButton) {
        payButton.addEventListener('click', async (e) => {
            e.preventDefault();
            
            const paymentId = localStorage.getItem("bp_paymentId");
            const amount = localStorage.getItem("bp_amount");
            const username = localStorage.getItem("bp_username") || "";
            const transactionId = `txn_${Date.now()}`;

            if (!paymentId || !amount) {
                alert('Payment ID or amount is missing.');
                return;
            }

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

                    // Redirect to processing page
                    window.location.href = 'processing.html';

                    // WebSocket connection for real-time updates
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
                    alert('Failed to initiate payment.');
                }
            } catch (error) {
                console.error('Error during payment:', error);
                alert('Error: ' + error.message);
            }
        });
    }
});
