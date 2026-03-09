#!/bin/bash

# Deploy updated Nginx configuration to backend server
echo "Deploying backend Nginx configuration..."

BACKEND_IP="18.190.155.165"
KEY="~/.ssh/flybird.pem"

# Copy the updated nginx config to the server
scp -i $KEY back/nginx.backend.conf ec2-user@$BACKEND_IP:/tmp/nginx.backend.conf

# SSH into the server and update Nginx
ssh -i $KEY ec2-user@$BACKEND_IP << 'EOF'
    echo "Backing up current config..."
    sudo cp /etc/nginx/conf.d/backend.conf /etc/nginx/conf.d/backend.conf.backup 2>/dev/null || true

    echo "Copying new config..."
    sudo cp /tmp/nginx.backend.conf /etc/nginx/conf.d/backend.conf

    echo "Testing Nginx configuration..."
    sudo nginx -t

    if [ $? -eq 0 ]; then
        echo "Nginx configuration is valid"
        sudo systemctl reload nginx
        echo "Nginx reloaded successfully"

        echo "Checking backend container status..."
        docker ps | grep backend

        echo "Testing health endpoint..."
        curl -s http://localhost:8081/api/health || echo "Health check failed"
    else
        echo "Nginx configuration test failed, rolling back..."
        sudo cp /etc/nginx/conf.d/backend.conf.backup /etc/nginx/conf.d/backend.conf 2>/dev/null
        sudo systemctl reload nginx
        exit 1
    fi
EOF

echo "Backend Nginx deployment completed!"
